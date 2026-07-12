package workbench

import "context"

type workspaceRepo interface {
	GetOrCreatePersonalWorkspace(context.Context, string) (Workspace, error)
	GetOwned(context.Context, string, string) (Workspace, error)
}
type Service struct {
	repo workspaceRepo
	tm   *TokenManager
}

func NewService(r workspaceRepo, tm *TokenManager) *Service { return &Service{r, tm} }
func (s *Service) Issue(ctx context.Context, uid, wid string) (TokenResponse, error) {
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
