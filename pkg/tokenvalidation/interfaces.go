package tokenvalidation

import userv1 "github.com/openshift/api/user/v1"

type UserToGroupMapper interface {
	GroupsFor(username string) ([]*userv1.Group, error)
}

type NoopGroupMapper struct{}

func (n NoopGroupMapper) GroupsFor(username string) ([]*userv1.Group, error) {
	return []*userv1.Group{}, nil
}
