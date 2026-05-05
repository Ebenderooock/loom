package mediainfo

// MediaInfo holds parsed media metadata extracted from a release name.
type MediaInfo struct {
	AudioCodec     string   `json:"audio_codec"`
	AudioChannels  string   `json:"audio_channels"`
	VideoCodec     string   `json:"video_codec"`
	Resolution     string   `json:"resolution"`
	HDR            string   `json:"hdr"`
	AudioLanguages []string `json:"audio_languages"`
	SubLanguages   []string `json:"sub_languages"`
	SubType        string   `json:"sub_type"`
	Source         string   `json:"source"`
}

// MediaPreferences defines user preferences for scoring releases.
type MediaPreferences struct {
	ID                    string   `json:"id"`
	PreferredAudioCodecs  []string `json:"preferred_audio"`
	PreferredSubLanguages []string `json:"preferred_sub_languages"`
	RequireSubtitles      bool     `json:"require_subtitles"`
	PreferHDR             bool     `json:"prefer_hdr"`
	PreferAtmos           bool     `json:"prefer_atmos"`
	CreatedAt             string   `json:"created_at,omitempty"`
	UpdatedAt             string   `json:"updated_at,omitempty"`
}
