# ADR 0026: Custom Formats Architecture & Radarr-Compatible Scoring

## Status
Accepted

## Context
Loom's Movies module requires a custom formats subsystem to match Radarr's functionality. Custom formats are flexible scoring rules that allow users to prioritize release attributes (codec, resolution, source, audio, etc.) beyond the rigid quality definitions. They are critical for matching user preferences (e.g., "prefer H.265 + 10-bit + TrueHD audio").

This design must be compatible with Radarr's custom format format and scoring model to ensure users migrating from Radarr can import their configurations.

## Decision
Implement custom formats using a three-layer architecture (Service, Repository, Handlers) with the following design principles:

### 1. **Filter Composition (AND Logic, Implicit)**
- All filters within a CustomFormat use implicit AND logic
- A release matches a CustomFormat only if ALL filters match
- No explicit OR/AND operators needed in Phase 5c
- Can be extended to support OR in Phase 5d if user demand exists

### 2. **Score Composition Formula**
```
FinalScore = (quality_tier_order × 100) + sum(matching_custom_format_scores)
```
- Quality tier provides base score (100 = lowest, 900 = highest)
- Custom format scores provide bonuses (+100) or penalties (-100)
- Release eligible if `quality_is_allowed AND (CustomFormatScore ≥ MinFormatScore OR MinFormatScore is 0)`

### 3. **Filter Conditions (8 Types)**
- **Equals**: Case-insensitive string equality (e.g., "x264")
- **Regex**: Perl-compatible regex pattern matching
- **Range**: Numeric range with optional bounds (e.g., "1,10" or ",100" for upper-bound only)
- **In**: Comma-separated list of allowed values
- **GreaterThan (gt)**: Numeric comparison >
- **GreaterThanOrEqual (gte)**: Numeric comparison ≥
- **LessThan (lt)**: Numeric comparison <
- **LessThanOrEqual (lte)**: Numeric comparison ≤

### 4. **Filter Fields (Allowed Allowlist)**
Only the following fields are supported in Phase 5c:
- `codec` — x264, x265, VP9, AV1, etc.
- `source` — Blu-ray, DVDRip, WebRip, HDTV, etc.
- `year` — Release year as numeric range
- `bitdepth` — 8, 10, 12 (numeric comparison)
- `resolution` — 480p, 720p, 1080p, 2160p
- `hdr` — HDR10, Dolby Vision, HLG (regex patterns)
- `audio` — TrueHD, DTS-X, Atmos, AAC, etc.
- `language` — ISO 639-1 codes (e.g., "en", "fr", "ja")

### 5. **Database Design**
- `custom_formats`: Header table with soft-delete support (id, name, description, tags, created_at, updated_at, deleted_at)
- `custom_format_filters`: Junction table (id, custom_format_id, field, condition, value, order, created_at, updated_at)
  - `order` column preserves filter evaluation sequence (for UI presentation and future OR support)
  - CASCADE delete on custom_format_id ensures atomicity
- Indexes on `deleted_at`, `created_at`, `custom_format_id`, `field` for query optimization

### 6. **Validation Rules**
- **Field allowlist**: Only allowed fields accepted; others rejected
- **Condition-specific validation**:
  - Regex patterns compiled and validated (catches invalid patterns early; no ReDoS timeout in Phase 5c but could be added in 5d)
  - Numeric conditions (gt/gte/lt/lte) validate that value is numeric
  - Range condition validates "min,max" format with numeric bounds (commas required; open bounds allowed)
  - In condition rejects empty lists
- **Filter count**: At least one filter required
- **Name**: Non-empty required

### 7. **Service Layer**
- `ValidateCustomFormat`: Pre-persistence validation (fields, conditions, syntax)
- `EvaluateCustomFormats`: Stateless evaluation against release name (returns matching formats with scores)
- `matchesAllFilters`: Internal helper for AND composition

### 8. **Repository Layer (CRUD)**
- `AddCustomFormat`: Insert header + iterate filters for individual INSERTs (not batch; simpler for Phase 5c)
- `GetCustomFormat`: Load format + all filters via custom_format_id
- `UpdateCustomFormat`: Delete existing filters + re-insert (simpler than merge logic)
- `DeleteCustomFormat`: Soft delete via updated_at timestamp
- `ListCustomFormats`: Load all IDs, fetch each individually (could be optimized in Phase 5d)
- `GetCustomFormatByName`: Return nil if not found (distinguishes "not found" from "error")

### 9. **HTTP Endpoints**
- `GET /api/v1/custom-formats` — List all custom formats
- `POST /api/v1/custom-formats` — Create format
- `GET /api/v1/custom-formats/{id}` — Get by ID
- `PUT /api/v1/custom-formats/{id}` — Update format
- `DELETE /api/v1/custom-formats/{id}` — Soft delete
- `POST /api/v1/custom-formats/test` — Test if release name matches filters

### 10. **Testing Strategy**
- Unit tests for ValidateCustomFormat (allowed fields, condition validation, edge cases)
- Unit tests for filterMatches (all 8 condition types, case sensitivity, regex handling)
- Fixture corpus of 100+ realistic release names with known-good matches (Phase 5d enhancement)
- ReDoS protection tests (prevent exponential backtracking in regex evaluation)

## Rationale
- **Implicit AND logic** simplifies Phase 5c scope and matches user mental model (common case)
- **Filter allowlist** prevents parser fragility (only validated fields are supported; others added per user request)
- **Soft deletes** maintain audit trail and enable recovery
- **Condition-specific validation** catches errors early (regex compilation, numeric parsing)
- **Stateless evaluation** (no DB access during matching) enables high-performance batch evaluation
- **Radarr compatibility**: Score formula, condition types, and field names match Radarr v4 API exactly

## Consequences

### Positive
- Clear, explicit validation rules prevent invalid custom format configurations
- Service layer becomes single source of truth for scoring logic
- Soft deletes enable audit compliance and recovery
- Condition-specific validation catches bugs before runtime
- Radarr migration scripts can reuse this validation layer

### Negative
- Filter composition is AND-only (Phase 5c); OR support requires design change
- Numeric conditions (gt/gte/lt/lte) require a release parser (implemented in Phase 5d)
- Single-insert filter loop (not batch) adds latency for formats with 10+ filters
- No caching of compiled regex patterns (could be optimized in Phase 5d)
- ListCustomFormats queries N formats individually (could use JOIN in Phase 5d)

### Unresolved
- Should filter field values be case-sensitive or case-insensitive? Currently inconsistent:
  - ConditionEquals is case-insensitive (via strings.EqualFold)
  - Others may vary; design decision needed
- Should CustomFormatScore scores be stored per-quality-profile or globally?
  - Current design assumes per-profile (FormatItems in QualityProfile)
  - Evaluation returns all matching formats without scores
- How should cascading deletes work for quality profiles that reference deleted custom formats?
  - Current design allows orphaning (soft-delete keeps references)
  - Migration/cleanup strategy TBD

## Future Work
- **Phase 5d**: Implement release-name parser to extract codec, resolution, bitdepth, etc.
  - Enables Range/GT/LT/GTE/LTE condition evaluation
  - Port Radarr's regex set and test against 10k+ release fixtures
- **Phase 5d**: Add caching/performance optimization for 100+ custom format sets
  - Cache compiled regex patterns in memory
  - Batch filter evaluation via optimized matching algorithm
- **Phase 5e**: Extend filter composition to support OR logic if user demand exists
  - Design: `(filter1 AND filter2) OR (filter3 AND filter4)` via groups
- **Phase 5e**: Custom format templates/presets for common scenarios
  - "Prefer H.265 + 10-bit + Atmos"
  - "Avoid multi-audio encodes"
  - "Only 1080p or better"

## Related ADRs
- ADR 0005: Quality Profiles & Definitions Architecture (integrates via FormatItems)
- Phase 5d: Release-name parser (required for numeric conditions)
