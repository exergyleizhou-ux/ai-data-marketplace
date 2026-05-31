// Package dataset owns dataset metadata, versions, files, chunked upload and
// the source-legality declaration / e-signature flow (来源合法性承诺).
//
// Boundary: owns `dataset`, `dataset_version`, `dataset_file`. Large objects
// live in object storage (OSS/COS) — only keys and metadata live in Postgres.
//
// Implemented in: PR-07 (metadata CRUD + source signing), PR-08 (chunked upload).
package dataset
