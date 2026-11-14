package imports

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"

	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/libraries"
	"github.com/ebenderooock/loom/internal/mediafiles"
	musiclib "github.com/ebenderooock/loom/internal/music"
)

func (p *ImportPipeline) processMusicImport(ctx context.Context, ev *downloads.DownloadCompletedEvent, downloadPath string, info os.FileInfo) ([]string, error) {
	if p.musicSvc == nil {
		return nil, p.recordFailure(ctx, string(MediaTypeMusic), "", ev.Title, downloadPath,
			fmt.Errorf("music import service is not configured"))
	}
	var files []string
	var err error
	if info.IsDir() {
		files, err = scanAudioFiles(ctx, downloadPath)
		if err != nil {
			return nil, p.recordFailure(ctx, string(MediaTypeMusic), "", ev.Title, downloadPath, err)
		}
	} else if mediafiles.IsAudio(strings.ToLower(filepath.Ext(downloadPath))) {
		files = []string{downloadPath}
	}
	if len(files) == 0 {
		return nil, p.recordFailure(ctx, string(MediaTypeMusic), "", ev.Title, downloadPath,
			fmt.Errorf("no audio files found in %s", downloadPath))
	}
	var failures []error
	var paths []string
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return paths, err
		}
		dest, err := p.importMusicFile(ctx, ev, file)
		if err != nil {
			p.logger.Error("failed to import music file", "file", file, "error", err)
			failures = append(failures, err)
			continue
		}
		paths = append(paths, dest)
	}
	if len(failures) > 0 {
		return paths, fmt.Errorf("music import incomplete (%d of %d files failed): %w", len(failures), len(files), errors.Join(failures...))
	}
	p.logger.Info("music import completed", "title", ev.Title, "imported", len(paths), "total", len(files))
	return paths, nil
}

func (p *ImportPipeline) importMusicFile(ctx context.Context, ev *downloads.DownloadCompletedEvent, source string) (string, error) {
	if p.musicSvc == nil {
		return "", p.recordFailure(ctx, string(MediaTypeMusic), "", ev.Title, source,
			fmt.Errorf("music import service is not configured"))
	}
	if dest, err := p.completedImport(ctx, source); err != nil {
		return "", err
	} else if dest != "" {
		if p.importMode == ImportModeMove && ev.DownloadID == "" {
			if err := p.FinalizeImports(ctx, []string{dest}); err != nil {
				return "", err
			}
		}
		return dest, nil
	}
	artist, album, track, tags, err := p.matchMusicFile(ctx, source, ev.Title)
	if err != nil {
		return "", p.recordFailure(ctx, string(MediaTypeMusic), "", ev.Title, source, err)
	}
	return p.importMatchedMusicFile(ctx, ev, source, artist, album, track, tags)
}

func (p *ImportPipeline) matchMusicFile(ctx context.Context, source, releaseTitle string) (*musiclib.Artist, *musiclib.Album, *musiclib.Track, *musiclib.AudioTags, error) {
	if p.musicSvc == nil {
		return nil, nil, nil, nil, fmt.Errorf("music import service is not configured")
	}
	tags, err := musiclib.ReadAudioTags(source)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("read audio tags: %w", err)
	}
	parsed := musiclib.ParseMusic(releaseTitle)
	artistName := strings.TrimSpace(tags.AlbumArtist)
	if artistName == "" {
		artistName = strings.TrimSpace(tags.Artist)
	}
	if artistName == "" {
		artistName = parsed.Artist
	}
	albumName := strings.TrimSpace(tags.Album)
	if albumName == "" {
		albumName = parsed.Album
	}
	trackArtistName := strings.TrimSpace(tags.Artist)
	if trackArtistName == "" {
		trackArtistName = artistName
	}
	artist, album, track, err := p.findMusicTrack(ctx, artistName, trackArtistName, albumName, tags)
	if err != nil {
		return nil, nil, nil, tags, err
	}
	return artist, album, track, tags, nil
}

func (p *ImportPipeline) importMatchedMusicFile(ctx context.Context, ev *downloads.DownloadCompletedEvent, source string, artist *musiclib.Artist, album *musiclib.Album, track *musiclib.Track, tags *musiclib.AudioTags) (string, error) {
	libraryID := album.LibraryID
	if album.ArtistID != "" {
		if artist == nil {
			return "", p.recordFailure(ctx, string(MediaTypeMusic), album.ID, ev.Title, source,
				fmt.Errorf("music album %q has no managed owner", album.Title))
		}
		libraryID = artist.LibraryID
	}
	if p.libStore == nil || libraryID == "" {
		return "", p.recordFailure(ctx, string(MediaTypeMusic), album.ID, ev.Title, source,
			fmt.Errorf("music album %q has no configured library", album.Title))
	}
	lib, err := p.libStore.Get(ctx, libraryID)
	if err != nil {
		return "", p.recordFailure(ctx, string(MediaTypeMusic), album.ID, ev.Title, source,
			fmt.Errorf("get music library: %w", err))
	}
	if lib.MediaType != "music" || strings.TrimSpace(lib.Path) == "" || !filepath.IsAbs(lib.Path) {
		return "", p.recordFailure(ctx, string(MediaTypeMusic), album.ID, ev.Title, source,
			fmt.Errorf("album %q must use an absolute music library path", album.Title))
	}
	multiDisc := false
	for _, candidate := range album.Tracks {
		if candidate.DiscNumber > 1 {
			multiDisc = true
			break
		}
	}
	relative := musiclib.BuildTrackPath(artist, album, track, multiDisc, strings.ToLower(filepath.Ext(source)))
	destination := filepath.Join(lib.Path, relative)
	if previous, err := p.completedImport(ctx, source); err != nil {
		return "", err
	} else if previous == destination {
		if p.importMode == ImportModeMove && ev.DownloadID == "" {
			if err := p.FinalizeImports(ctx, []string{destination}); err != nil {
				return "", err
			}
		}
		return destination, nil
	}
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return "", p.recordFailure(ctx, string(MediaTypeMusic), album.ID, ev.Title, source,
			fmt.Errorf("inspect source audio file: %w", err))
	}
	bitrate := tags.Bitrate
	if bitrate <= 0 {
		bitrate = musiclib.EstimateAudioBitrate(sourceInfo.Size(), track.DurationMs)
	}
	oldTrackFiles, err := p.prepareMusicUpgrade(ctx, artist, album, track, lib.Path, destination, source, tags, bitrate)
	if err != nil {
		return "", p.recordFailure(ctx, string(MediaTypeMusic), album.ID, ev.Title, source, err)
	}
	if filepath.Clean(source) != filepath.Clean(destination) {
		// Stage moves without consuming the source until registration succeeds;
		// FinalizeImports removes download sources afterwards.
		mode := p.importMode
		if mode == ImportModeMove {
			mode = ImportModeHardlink
		}
		if err := importFile(source, destination, mode); err != nil {
			return "", p.recordFailure(ctx, string(MediaTypeMusic), album.ID, ev.Title, source,
				fmt.Errorf("import audio file: %w", err))
		}
	}
	info, err := os.Stat(destination)
	if err != nil {
		return "", p.recordFailure(ctx, string(MediaTypeMusic), album.ID, ev.Title, source,
			fmt.Errorf("inspect imported audio file: %w", err))
	}
	if p.verifier != nil {
		if result := p.verifier.Verify(destination, sourceInfo.Size()); !result.OK {
			return "", p.recordFailure(ctx, string(MediaTypeMusic), album.ID, ev.Title, source,
				fmt.Errorf("verify imported audio file (source preserved): %s", result.Reason))
		}
	}
	var fileDate *time.Time
	modified := info.ModTime()
	fileDate = &modified
	mediaInfo := musiclib.MediaInfoMap{}
	if tags.Genre != "" {
		mediaInfo["genre"] = tags.Genre
	}
	if tags.Year > 0 {
		mediaInfo["year"] = tags.Year
	}
	if bitrate > 0 {
		mediaInfo["bitrate"] = bitrate
	}
	if tags.Bits > 0 {
		mediaInfo["bits"] = tags.Bits
	}
	tf := &musiclib.TrackFile{
		ID: uuid.New().String(), TrackID: track.ID, AlbumID: album.ID,
		FilePath: destination, Size: info.Size(), Quality: tags.Format, Format: tags.Format,
		Bitrate: bitrate, MediaInfo: mediaInfo, FileDate: fileDate,
	}
	if album.ArtistID != "" && artist != nil {
		tf.ArtistID = artist.ID
	}
	// A replacement can reuse the same pathname. Remove recycled file records
	// before registration so ImportTrackFile does not mistake the new file for
	// the old one and then leave the track without a file.
	for _, old := range oldTrackFiles {
		if err := p.musicSvc.DeleteTrackFile(ctx, old.ID); err != nil {
			return "", p.recordFailure(ctx, string(MediaTypeMusic), album.ID, ev.Title, destination,
				fmt.Errorf("remove replaced music track-file record %s: %w", old.ID, err))
		}
	}
	if err := p.musicSvc.ImportTrackFile(ctx, tf); err != nil {
		return "", p.recordFailure(ctx, string(MediaTypeMusic), album.ID, ev.Title, source,
			fmt.Errorf("save music track file (source preserved): %w", err))
	}
	mediaID := track.ID
	if err := p.libStore.UpsertFile(ctx, &libraries.LibraryFile{
		LibraryID: libraryID, Path: destination, SizeBytes: info.Size(), MediaID: &mediaID,
	}); err != nil {
		return "", p.recordFailure(ctx, string(MediaTypeMusic), album.ID, ev.Title, destination,
			fmt.Errorf("update music library index: %w", err))
	}
	if err := p.recordStatus(ctx, string(MediaTypeMusic), album.ID, source, destination, StatusImported, ""); err != nil {
		return "", fmt.Errorf("record import history (source preserved at %s): %w", source, err)
	}
	if p.importMode == ImportModeMove && ev.DownloadID == "" {
		if err := p.FinalizeImports(ctx, []string{destination}); err != nil {
			return "", fmt.Errorf("music import registered but source cleanup failed: %w", err)
		}
	}
	_ = p.bus.Publish(ctx, &ImportCompletedEvent{
		MediaType: MediaTypeMusic, MediaID: album.ID, Title: album.Title, DestPath: destination,
	})
	return destination, nil
}

func (p *ImportPipeline) importMusicMatch(ctx context.Context, source, trackID string) (*ImportRecord, error) {
	if p.musicSvc == nil {
		return nil, fmt.Errorf("music import service is not configured")
	}
	if err := checkRegularSource(source); err != nil {
		return nil, err
	}
	if !mediafiles.IsAudio(strings.ToLower(filepath.Ext(source))) {
		return nil, fmt.Errorf("not an audio file: %s", source)
	}
	track, err := p.musicSvc.GetTrack(ctx, trackID)
	if err != nil {
		return nil, fmt.Errorf("get music track: %w", err)
	}
	album, err := p.musicSvc.LoadAlbumTracks(ctx, track.AlbumID)
	if err != nil {
		return nil, fmt.Errorf("load music album: %w", err)
	}
	// Loading an edition can replace its track IDs; do not import a stale
	// selection into a track that is no longer in the selected edition.
	var selected *musiclib.Track
	for _, candidate := range album.Tracks {
		if candidate.ID == trackID {
			selected = candidate
			break
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("music track %s is not in the album's selected edition", trackID)
	}
	var artist *musiclib.Artist
	if album.ArtistID != "" {
		artist, err = p.musicSvc.GetArtist(ctx, album.ArtistID)
		if err != nil {
			return nil, fmt.Errorf("get music album owner: %w", err)
		}
	}
	tags, err := musiclib.ReadAudioTags(source)
	if err != nil {
		return nil, fmt.Errorf("read audio tags: %w", err)
	}
	ev := &downloads.DownloadCompletedEvent{Title: filepath.Base(source)}
	dest, err := p.importMatchedMusicFile(ctx, ev, source, artist, album, selected, tags)
	if err != nil {
		return nil, err
	}
	return &ImportRecord{
		ID: uuid.New().String(), ImportedAt: time.Now().UTC(),
		MediaType: MediaTypeMusic, MediaID: album.ID, SourcePath: source,
		DestPath: dest, ImportMode: p.importMode, Status: StatusImported,
	}, nil
}

func (p *ImportPipeline) scanMusicFile(ctx context.Context, source string) ScanResult {
	result := ScanResult{
		FilePath: source, MediaType: string(MediaTypeMusic),
		DetectedTitle:   strings.TrimSuffix(filepath.Base(source), filepath.Ext(source)),
		SuggestedAction: "no_match",
	}
	if info, err := os.Stat(source); err == nil {
		result.FileSize = info.Size()
	}
	_, album, track, tags, err := p.matchMusicFile(ctx, source, filepath.Base(filepath.Dir(source)))
	if tags != nil {
		if tags.Title != "" {
			result.DetectedTitle = tags.Title
		}
		result.DetectedArtist = tags.Artist
		result.DetectedAlbum = tags.Album
		result.DetectedTrack = tags.TrackNumber
		result.DetectedDisc = tags.DiscNumber
		result.DetectedYear = tags.Year
		result.Quality = tags.Format
	}
	if err != nil {
		result.ImportError = err.Error()
		result.SuggestedAction = "manual_review"
		p.logger.Warn("music scan match failed", "file", source, "error", err)
		return result
	}
	result.MatchedMedia = fmt.Sprintf("%s / %s", album.Title, track.Title)
	result.MatchedMediaID = track.ID
	result.Confidence = 1
	result.SuggestedAction = "import"
	if track.HasFile {
		result.SuggestedAction = "reimport"
	}
	return result
}

func (p *ImportPipeline) prepareMusicUpgrade(
	ctx context.Context,
	artist *musiclib.Artist,
	album *musiclib.Album,
	track *musiclib.Track,
	libraryRoot, destination, source string,
	tags *musiclib.AudioTags,
	bitrate int,
) ([]*musiclib.TrackFile, error) {
	var files []*musiclib.TrackFile
	var err error
	if album.ArtistID != "" && artist != nil {
		files, err = p.musicSvc.ListTrackFilesByArtist(ctx, artist.ID)
	} else {
		files, err = p.musicSvc.ListTrackFilesByAlbum(ctx, album.ID)
	}
	if err != nil {
		return nil, fmt.Errorf("list existing music track files: %w", err)
	}
	var existing []*musiclib.TrackFile
	for _, file := range files {
		if file.TrackID == track.ID {
			existing = append(existing, file)
		}
	}
	if len(existing) == 0 {
		return nil, nil
	}
	if filepath.Clean(source) == filepath.Clean(destination) {
		return nil, nil
	}
	for _, file := range existing {
		if filepath.Clean(file.FilePath) == filepath.Clean(source) {
			return nil, nil
		}
	}

	definitions, err := p.musicSvc.ListAudioQualityDefinitions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list audio quality definitions: %w", err)
	}
	incoming := musiclib.MatchAudioQuality(&musiclib.MusicRelease{
		Format: tags.Format, Bitrate: bitrate, Bits: tags.Bits,
	}, definitions)
	if incoming == nil {
		return nil, fmt.Errorf("cannot determine quality for incoming track %q; preserving existing file", track.Title)
	}
	highestExistingTier := -1
	for _, file := range existing {
		release := &musiclib.MusicRelease{Format: file.Format, Bitrate: file.Bitrate}
		switch bits := file.MediaInfo["bits"].(type) {
		case int:
			release.Bits = bits
		case int64:
			release.Bits = int(bits)
		case float64:
			release.Bits = int(bits)
		}
		if definition := musiclib.MatchAudioQuality(release, definitions); definition != nil &&
			definition.TierOrder > highestExistingTier {
			highestExistingTier = definition.TierOrder
		}
	}
	if incoming.TierOrder <= highestExistingTier {
		return nil, fmt.Errorf("incoming quality %q does not improve existing track %q; preserving existing file", incoming.Name, track.Title)
	}
	if p.recycler == nil || !p.recycler.Enabled || p.recycler.Path == "" {
		return nil, fmt.Errorf("cannot replace existing track %q without an enabled recycle bin", track.Title)
	}

	if _, err := os.Lstat(destination); err == nil {
		knownDestination := false
		for _, old := range existing {
			if filepath.Clean(old.FilePath) == filepath.Clean(destination) {
				knownDestination = true
				break
			}
		}
		if !knownDestination {
			return nil, fmt.Errorf("refusing to overwrite unrelated music file %s", destination)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect music destination: %w", err)
	}

	for _, old := range existing {
		if _, err := os.Lstat(old.FilePath); err == nil {
			if err := p.recycler.Recycle(old.FilePath, libraryRoot); err != nil {
				return nil, fmt.Errorf("preserve existing track %q before upgrade: %w", track.Title, err)
			}
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("inspect existing track %q: %w", track.Title, err)
		}
	}
	return existing, nil
}

func (p *ImportPipeline) findMusicTrack(ctx context.Context, albumArtistName, trackArtistName, albumName string, tags *musiclib.AudioTags) (*musiclib.Artist, *musiclib.Album, *musiclib.Track, error) {
	if (strings.TrimSpace(albumArtistName) == "" && strings.TrimSpace(trackArtistName) == "") ||
		strings.TrimSpace(albumName) == "" || tags == nil {
		return nil, nil, nil, fmt.Errorf("audio tags do not identify an artist and album")
	}
	effectiveTags := *tags
	if strings.TrimSpace(effectiveTags.Artist) == "" {
		effectiveTags.Artist = trackArtistName
	}
	artists, err := p.musicSvc.ListArtists(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	artistsByID := make(map[string]*musiclib.Artist, len(artists))
	for _, candidate := range artists {
		artistsByID[candidate.ID] = candidate
	}
	albums, err := p.musicSvc.ListAlbums(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	type albumTrackMatch struct {
		album *musiclib.Album
		full  *musiclib.Album
		track *musiclib.Track
		owner *musiclib.Artist
	}
	var matches []albumTrackMatch
	var sameTitle []*musiclib.Album
	var ambiguousTracks []string
	var metadataErrors []error
	for _, candidate := range albums {
		editionFallback := normalizeMusicText(candidate.Title) != normalizeMusicText(albumName)
		if editionFallback &&
			normalizeMusicText(musiclib.BaseAlbumTitle(candidate.Title)) != normalizeMusicText(musiclib.BaseAlbumTitle(albumName)) {
			continue
		}
		sameTitle = append(sameTitle, candidate)
		owner := artistsByID[candidate.ArtistID]
		identityEstablished := albumCreditMatches(candidate, owner, albumArtistName)
		if !identityEstablished && strings.TrimSpace(albumArtistName) == "" &&
			!musiclib.IsVariousArtistAlbum(candidate) {
			identityEstablished = albumCreditMatches(candidate, owner, trackArtistName)
		}
		if !identityEstablished && !musiclib.IsVariousArtistAlbum(candidate) {
			continue
		}
		loaded, loadErr := p.musicSvc.LoadAlbumTracks(ctx, candidate.ID)
		if loadErr != nil {
			ambiguousTracks = append(ambiguousTracks, fmt.Sprintf("%s (track metadata unavailable)", candidate.ID))
			metadataErrors = append(metadataErrors, fmt.Errorf("load album %q: %w", candidate.Title, loadErr))
			continue
		}
		candidateTags := effectiveTags
		if editionFallback {
			// Track positions are edition-specific, not recording identity.
			candidateTags.TrackNumber = 0
			candidateTags.DiscNumber = 0
		}
		candidateTrack, matchErr := matchTaggedTrack(loaded.Tracks, &candidateTags, !identityEstablished)
		if matchErr != nil {
			ambiguousTracks = append(ambiguousTracks, candidate.ID)
			continue
		}
		if candidateTrack != nil && loaded.SelectedReleaseID != "" &&
			candidateTrack.ReleaseID != "" && candidateTrack.ReleaseID != loaded.SelectedReleaseID {
			ambiguousTracks = append(ambiguousTracks, fmt.Sprintf("%s (track belongs to a different selected release)", candidate.ID))
			continue
		}
		if candidateTrack != nil {
			matches = append(matches, albumTrackMatch{
				album: candidate, full: loaded, track: candidateTrack, owner: owner,
			})
		}
	}
	if len(matches) > 1 {
		ids := make([]string, 0, len(matches))
		for _, match := range matches {
			ids = append(ids, match.album.ID)
		}
		return nil, nil, nil, fmt.Errorf("ambiguous music album %q: matching albums %s", albumName, strings.Join(ids, ", "))
	}
	if len(matches) == 1 && len(ambiguousTracks) > 0 {
		return nil, nil, nil, errors.Join(
			fmt.Errorf("ambiguous music album %q: track identity could not be verified for %s", albumName, strings.Join(ambiguousTracks, ", ")),
			errors.Join(metadataErrors...))
	}
	if len(matches) == 0 {
		if len(ambiguousTracks) > 0 {
			if len(metadataErrors) > 0 {
				return nil, nil, nil, errors.Join(metadataErrors...)
			}
			return nil, nil, nil, fmt.Errorf("ambiguous music track %q on album %q: matching track identity is unavailable or duplicated", tags.Title, albumName)
		}
		if len(sameTitle) > 0 {
			return nil, nil, nil, fmt.Errorf("music album %q has no matching credited artist and track identity for %q; matching selected-release metadata is required", albumName, tags.Title)
		}
		return nil, nil, nil, fmt.Errorf("could not match audio track %q on album %q", tags.Title, albumName)
	}
	match := matches[0]
	return match.owner, match.full, match.track, nil
}

func albumCreditMatches(album *musiclib.Album, owner *musiclib.Artist, name string) bool {
	name = normalizeMusicText(name)
	if name == "" || name == normalizeMusicText("Various Artists") {
		return false
	}
	if owner != nil && normalizeMusicText(owner.Name) == name {
		return true
	}
	for _, credit := range album.Artists {
		if normalizeMusicText(credit.Name) == name {
			return true
		}
	}
	return false
}

func matchTaggedTrack(tracks []*musiclib.Track, tags *musiclib.AudioTags, requireArtist bool) (*musiclib.Track, error) {
	var candidates []*musiclib.Track
	if strings.TrimSpace(tags.Title) != "" {
		title := normalizeMusicText(tags.Title)
		var eligible []*musiclib.Track
		for _, candidate := range tracks {
			if candidate == nil || !trackArtistMatches(candidate, tags.Artist, requireArtist) {
				continue
			}
			eligible = append(eligible, candidate)
			if normalizeMusicText(candidate.Title) == title {
				candidates = append(candidates, candidate)
			}
		}
		if len(candidates) == 0 {
			title = normalizeMusicText(musicTrackIdentityTitle(tags.Title))
			for _, candidate := range eligible {
				if title != "" && normalizeMusicText(musicTrackIdentityTitle(candidate.Title)) == title {
					candidates = append(candidates, candidate)
				}
			}
		}
	} else if !requireArtist && tags.TrackNumber > 0 {
		disc := tags.DiscNumber
		if disc < 1 {
			disc = 1
		}
		for _, candidate := range tracks {
			if candidate == nil || candidate.TrackNumber != tags.TrackNumber {
				continue
			}
			candidateDisc := candidate.DiscNumber
			if candidateDisc < 1 {
				candidateDisc = 1
			}
			if candidateDisc == disc && trackArtistMatches(candidate, tags.Artist, false) {
				candidates = append(candidates, candidate)
			}
		}
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	disc := tags.DiscNumber
	if disc < 1 {
		disc = 1
	}
	var positioned []*musiclib.Track
	for _, candidate := range candidates {
		candidateDisc := candidate.DiscNumber
		if candidateDisc < 1 {
			candidateDisc = 1
		}
		if tags.TrackNumber > 0 && candidate.TrackNumber == tags.TrackNumber && candidateDisc == disc {
			positioned = append(positioned, candidate)
		}
	}
	if len(positioned) == 1 {
		return positioned[0], nil
	}
	return nil, fmt.Errorf("multiple tracks match title and artist")
}

var musicTrackCreditSuffix = regexp.MustCompile(`(?i)\s*(?:\((?:feat\.?|ft\.?|featuring)\s+[^()]+\)|\[(?:feat\.?|ft\.?|featuring)\s+[^\[\]]+\]|\s+(?:feat\.?|ft\.?|featuring)\s+[^\(\)\[\]]+)\s*$`)

// Credit spelling may vary; recording/version qualifiers must remain intact.
func musicTrackIdentityTitle(title string) string {
	base := strings.TrimSpace(title)
	for {
		next := strings.TrimSpace(musicTrackCreditSuffix.ReplaceAllString(base, ""))
		if next == "" || next == base {
			return base
		}
		base = next
	}
}

func trackArtistMatches(track *musiclib.Track, artistName string, requireArtist bool) bool {
	artistName = normalizeMusicText(artistName)
	trackArtist := normalizeMusicText(track.ArtistName)
	if requireArtist {
		return artistName != "" && trackArtist != "" && artistName == trackArtist
	}
	return artistName == "" || trackArtist == "" || artistName == trackArtist
}

func scanAudioFiles(ctx context.Context, dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() && mediafiles.IsAudio(strings.ToLower(filepath.Ext(path))) {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func normalizeMusicText(value string) string {
	var builder strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(value)) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}
