package imports

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	scanPath := downloadPath
	if !info.IsDir() {
		scanPath = filepath.Dir(downloadPath)
	}
	files, err := scanAudioFiles(ctx, scanPath)
	if err != nil {
		return nil, p.recordFailure(ctx, string(MediaTypeMusic), "", ev.Title, downloadPath, err)
	}
	if !info.IsDir() && mediafiles.IsAudio(strings.ToLower(filepath.Ext(downloadPath))) {
		files = []string{downloadPath}
	}
	if len(files) == 0 {
		return nil, p.recordFailure(ctx, string(MediaTypeMusic), "", ev.Title, downloadPath,
			fmt.Errorf("no audio files found in %s", scanPath))
	}
	var lastErr error
	var paths []string
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return paths, err
		}
		dest, err := p.importMusicFile(ctx, ev, file)
		if err != nil {
			p.logger.Error("failed to import music file", "file", file, "error", err)
			lastErr = err
			continue
		}
		paths = append(paths, dest)
	}
	if len(paths) == 0 && lastErr != nil {
		return nil, lastErr
	}
	p.logger.Info("music import completed", "title", ev.Title, "imported", len(paths), "total", len(files))
	return paths, nil
}

func (p *ImportPipeline) importMusicFile(ctx context.Context, ev *downloads.DownloadCompletedEvent, source string) (string, error) {
	tags, err := musiclib.ReadAudioTags(source)
	if err != nil {
		return "", p.recordFailure(ctx, string(MediaTypeMusic), "", ev.Title, source, fmt.Errorf("read audio tags: %w", err))
	}
	parsed := musiclib.ParseMusic(ev.Title)
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
		return "", p.recordFailure(ctx, string(MediaTypeMusic), "", ev.Title, source, err)
	}
	libraryID := album.LibraryID
	if album.ArtistID != "" && artist != nil && artist.LibraryID != "" {
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
	multiDisc := false
	for _, candidate := range album.Tracks {
		if candidate.DiscNumber > 1 {
			multiDisc = true
			break
		}
	}
	relative := musiclib.BuildTrackPath(artist, album, track, multiDisc, strings.ToLower(filepath.Ext(source)))
	destination := filepath.Join(lib.Path, relative)
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
	if err := p.musicSvc.ImportTrackFile(ctx, tf); err != nil {
		return "", p.recordFailure(ctx, string(MediaTypeMusic), album.ID, ev.Title, destination,
			fmt.Errorf("save music track file: %w", err))
	}
	for _, old := range oldTrackFiles {
		if err := p.musicSvc.DeleteTrackFile(ctx, old.ID); err != nil {
			return "", p.recordFailure(ctx, string(MediaTypeMusic), album.ID, ev.Title, destination,
				fmt.Errorf("remove replaced music track-file record %s: %w", old.ID, err))
		}
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
	if p.importMode == ImportModeMove && ev.DownloadID == "" && filepath.Clean(source) != filepath.Clean(destination) {
		if err := os.Remove(source); err != nil && !os.IsNotExist(err) {
			p.logger.Warn("music import registered but source cleanup failed", "source", source, "error", err)
		}
	}
	_ = p.bus.Publish(ctx, &ImportCompletedEvent{
		MediaType: MediaTypeMusic, MediaID: album.ID, Title: album.Title, DestPath: destination,
	})
	return destination, nil
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
	for _, candidate := range albums {
		if normalizeMusicText(candidate.Title) != normalizeMusicText(albumName) {
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
			continue
		}
		candidateTrack, matchErr := matchTaggedTrack(loaded.Tracks, &effectiveTags, !identityEstablished)
		if matchErr != nil {
			ambiguousTracks = append(ambiguousTracks, candidate.ID)
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
		return nil, nil, nil, fmt.Errorf("ambiguous music album %q: track identity could not be verified for %s", albumName, strings.Join(ambiguousTracks, ", "))
	}
	if len(matches) == 0 {
		if len(ambiguousTracks) > 0 {
			return nil, nil, nil, fmt.Errorf("ambiguous music track %q on album %q: matching track identity is unavailable or duplicated", tags.Title, albumName)
		}
		if len(sameTitle) > 0 {
			return nil, nil, nil, fmt.Errorf("music album %q has no matching credited artist and track identity", albumName)
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
			if track := musiclib.MatchTrackByBaseTitle(eligible, tags.Title, normalizeMusicText); track != nil {
				candidates = append(candidates, track)
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
