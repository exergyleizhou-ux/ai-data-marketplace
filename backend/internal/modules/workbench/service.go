package workbench

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

type workspaceRepo interface {
	AccountActive(context.Context, string) (bool, error)
	GetOrCreatePersonalWorkspace(context.Context, string) (Workspace, error)
	GetOwned(context.Context, string, string) (Workspace, error)
}
type Service struct {
	repo    workspaceRepo
	tm      *TokenManager
	runtime *Repository
	objects storage.Storage
}

func NewService(r workspaceRepo, tm *TokenManager) *Service { return &Service{repo: r, tm: tm} }
func NewManagedService(r *Repository, tm *TokenManager, objects storage.Storage) *Service {
	return &Service{repo: r, tm: tm, runtime: r, objects: objects}
}
func (s *Service) Issue(ctx context.Context, uid, wid string) (TokenResponse, error) {
	active, err := s.repo.AccountActive(ctx, uid)
	if err != nil {
		return TokenResponse{}, err
	}
	if !active {
		return TokenResponse{}, ErrAccountInactive
	}
	var w Workspace
	var e error
	if wid == "" {
		w, e = s.repo.GetOrCreatePersonalWorkspace(ctx, uid)
	} else {
		w, e = s.repo.GetOwned(ctx, wid, uid)
	}
	if e != nil {
		return TokenResponse{}, e
	}
	t, ttl, e := s.tm.Issue(uid, w.ID)
	return TokenResponse{t, ttl, w.ID}, e
}

func (s *Service) owner(ctx context.Context, uid, wid string) (Owner, error) {
	if wid == "" {
		return Owner{}, ErrNotFound
	}
	w, err := s.repo.GetOwned(ctx, wid, uid)
	if err != nil {
		return Owner{}, err
	}
	return Owner{AccountID: uid, WorkspaceID: w.ID}, nil
}
func (s *Service) Runs(ctx context.Context, uid, wid string, limit int) ([]Run, error) {
	o, e := s.owner(ctx, uid, wid)
	if e != nil {
		return nil, e
	}
	return s.runtime.ListRuns(ctx, o, limit)
}
func (s *Service) Run(ctx context.Context, uid, wid, id string) (Run, error) {
	o, e := s.owner(ctx, uid, wid)
	if e != nil {
		return Run{}, e
	}
	return s.runtime.GetRun(ctx, o, id)
}
func (s *Service) Events(ctx context.Context, uid, wid, id string, after int64) ([]Event, error) {
	o, e := s.owner(ctx, uid, wid)
	if e != nil {
		return nil, e
	}
	return s.runtime.Events(ctx, o, id, after, 200)
}
func (s *Service) Approvals(ctx context.Context, uid, wid, id string) ([]Approval, error) {
	o, e := s.owner(ctx, uid, wid)
	if e != nil {
		return nil, e
	}
	return s.runtime.ListApprovals(ctx, o, id)
}
func (s *Service) Decide(ctx context.Context, uid, wid, id string, d ApprovalDecision) (Approval, error) {
	o, e := s.owner(ctx, uid, wid)
	if e != nil {
		return Approval{}, e
	}
	return s.runtime.DecideApproval(ctx, o, id, uid, d.Decision, d.Version)
}
func (s *Service) Artifacts(ctx context.Context, uid, wid, id string) ([]Artifact, error) {
	o, e := s.owner(ctx, uid, wid)
	if e != nil {
		return nil, e
	}
	return s.runtime.ListArtifacts(ctx, o, id)
}
func (s *Service) OpenArtifact(ctx context.Context, uid, wid, id string) (Artifact, io.ReadCloser, int64, error) {
	o, e := s.owner(ctx, uid, wid)
	if e != nil {
		return Artifact{}, nil, 0, e
	}
	a, e := s.runtime.GetArtifact(ctx, o, id)
	if e != nil {
		return Artifact{}, nil, 0, e
	}
	if s.objects == nil {
		return Artifact{}, nil, 0, errors.New("object storage unavailable")
	}
	r, n, e := s.objects.Open(ctx, a.ObjectKey)
	return a, r, n, e
}

// ArtifactObjectKey is the sole key constructor for managed runtime output.
// Callers provide identifiers, never an object-store path.
func ArtifactObjectKey(o Owner, runID, artifactID string) (string, error) {
	for _, v := range []string{o.AccountID, o.WorkspaceID, runID, artifactID} {
		if v == "" || strings.ContainsAny(v, "/\\") || v == "." || v == ".." {
			return "", errors.New("invalid artifact identifier")
		}
	}
	return "workbench/" + o.AccountID + "/" + o.WorkspaceID + "/" + runID + "/" + artifactID, nil
}
