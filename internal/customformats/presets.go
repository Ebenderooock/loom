package customformats

// Presets returns the built-in custom format presets that users can
// import with one click.
func Presets() []CustomFormat {
	return []CustomFormat{
		{
			ID:   "prefer-hevc",
			Name: "Prefer x265/HEVC",
			Specifications: []Specification{
				{
					Name:           "x265/HEVC",
					Implementation: ImplCodec,
					Fields:         map[string]any{"value": "x265"},
				},
			},
		},
		{
			ID:   "prefer-atmos-truehd",
			Name: "Prefer Atmos/TrueHD",
			Specifications: []Specification{
				{
					Name:           "Atmos",
					Implementation: ImplAudio,
					Fields:         map[string]any{"value": "Atmos"},
				},
				{
					Name:           "TrueHD",
					Implementation: ImplAudio,
					Fields:         map[string]any{"value": "TrueHD"},
				},
			},
		},
		{
			ID:   "avoid-lq-groups",
			Name: "Avoid LQ Groups",
			Specifications: []Specification{
				{
					Name:           "LQ Release Group",
					Implementation: ImplReleaseTitle,
					Fields:         map[string]any{"value": `(?i)\b(YIFY|YTS|EVO|SPARKS|RARBG|aXXo|KORSUB|STUTTERSHIT|TGx)\b`},
				},
			},
		},
		{
			ID:   "prefer-bluray",
			Name: "Prefer BluRay",
			Specifications: []Specification{
				{
					Name:           "BluRay Source",
					Implementation: ImplSource,
					Fields:         map[string]any{"value": "BluRay"},
				},
			},
		},
		{
			ID:   "avoid-cam-ts",
			Name: "Avoid CAM/TS",
			Specifications: []Specification{
				{
					Name:           "CAM",
					Implementation: ImplSource,
					Fields:         map[string]any{"value": "CAM"},
				},
				{
					Name:           "TS",
					Implementation: ImplSource,
					Fields:         map[string]any{"value": "TS"},
				},
			},
		},
	}
}
