# ADR 0005: Quality Profiles & Definitions Architecture

## Status
Accepted

## Context
Loom's Movies module requires a quality management subsystem to match Radarr's functionality. Quality definitions define the technical properties of release tiers (resolution, source, codec), while quality profiles are named collections of quality definitions with cutoff and upgrade rules.

## Decision
Implement quality definitions and profiles using a three-layer architecture (Service, Repository, Handlers) with the following design principles:

1. **Quality Definitions (Immutable Metadata)**
   - Stored as reference data defining quality tiers (e.g., "1080p BluRay", "720p WebRip")
   - Fields: ID, Name, Title, Source, Resolution, Modifier, file size bounds, preferred ranking
   - Soft-deleted via `deleted_at` timestamp for audit trails

2. **Quality Profiles (Named Collections with Rules)**
   - Combine quality definitions into user-created policies
   - Critical validation: **Cutoff must exist in Items AND be marked Allowed**
   - Fields: ID, Name, Cutoff (quality def ID), UpgradeAllowed, Language, MinFormatScore, CutoffFormatScore
   - Items stored in junction table (`quality_profile_items`) with `preferred` and `allowed` flags

3. **Validation at Service Layer**
   - Cutoff consistency validation prevents invalid states (e.g., upgrade target not allowed)
   - Timestamps (CreatedAt/UpdatedAt) set at service layer before persistence
   - Soft delete filtering applied at repository query layer

4. **HTTP Endpoints (RESTful)**
   - 5 endpoints for quality definitions (POST, GET by ID, GET list, PATCH, DELETE)
   - 5 endpoints for quality profiles (POST, GET by ID, GET list, PATCH, DELETE)
   - HTTP status codes: 201 Created, 204 No Content, 400 Bad Request

5. **Database Design**
   - `quality_definitions`: Core lookup table with soft-delete support
   - `quality_profiles`: Header table with top-level profile config
   - `quality_profile_items`: Junction table linking profiles to definitions (with allowed/preferred flags)
   - Indexes on commonly-filtered columns (name, cutoff)

## Rationale
- **Layered architecture** (Service/Repository/Handlers) maintains separation of concerns and enables testing
- **Cutoff validation** at service layer ensures profiles are always in valid state (Radarr compatibility)
- **Soft deletes** preserve audit history and allow recovery without data loss
- **Junction table design** allows flexible association of definitions with multiple profiles
- **JSON serialization** for FormatItems enables forward compatibility with custom format scoring (Phase 5c)

## Consequences
- **Positive:**
  - Clear validation rules prevent invalid profile configurations
  - Service layer becomes single source of truth for business logic
  - Soft deletes enable audit compliance and recovery
  - Extensible to custom formats via FormatItems placeholder

- **Negative:**
  - Extra validation layer adds minor latency
  - Junction table requires transactions for atomic profile+items inserts
  - FormatItems (map[string]interface{}) is placeholder pending Phase 5c custom formats design

## Future Work
- Phase 5c: Implement custom format definitions and scoring rules to populate FormatItems
- Persist user-created quality definitions vs. system defaults (metadata versioning)
- Quality profile versioning and rollback
