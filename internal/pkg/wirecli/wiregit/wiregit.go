package wiregit

import (
	"github.com/google/wire"
	gogit "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
)

// OptionalProviderSet provides optional Git dependencies.
var OptionalProviderSet = wire.NewSet(NewOptionalRepository, NewOptionalCommitIter, NewOptionalCommit)

// NewOptionalRepository provides a *git.Repository or nil
func NewOptionalRepository(path clik8s.ResourceConfigPath) *gogit.Repository {
	r, _ := gogit.PlainOpen(string(path))
	return r
}

// NewOptionalCommitIter provides an object.CommitIter or nil
func NewOptionalCommitIter(r *gogit.Repository) object.CommitIter {
	if r == nil {
		return nil
	}
	i, _ := r.CommitObjects()
	return i
}

// NewOptionalCommit provides an *object.Commit or nil
func NewOptionalCommit(i object.CommitIter) *object.Commit {
	if i == nil {
		return nil
	}
	c, _ := i.Next()
	return c
}

// RequiredProviderSet provides required Git dependencies.
var RequiredProviderSet = wire.NewSet(NewRequiredRepository, NewRequiredCommitIter, NewRequiredCommit)

// NewRequiredRepository provides a *git.Repository or error
func NewRequiredRepository(path clik8s.ResourceConfigPath) (*gogit.Repository, error) {
	return gogit.PlainOpen(string(path))
}

// NewRequiredCommitIter provides an object.CommitIter or error
func NewRequiredCommitIter(r *gogit.Repository) (object.CommitIter, error) {
	return r.Log(&gogit.LogOptions{})
}

// NewRequiredCommit provides an *object.Commit or error
func NewRequiredCommit(i object.CommitIter) (*object.Commit, error) {
	return i.Next()
}
