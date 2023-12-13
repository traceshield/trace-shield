package model

import (
	"context"

	rts "github.com/ory/keto/proto/ory/keto/relation_tuples/v1alpha2"
	"github.com/traceshield/trace-shield/consts"
	"github.com/traceshield/trace-shield/graph/common"
	"github.com/traceshield/trace-shield/graph/resolvers/helpers"
)

// function that processes a relation tuple and returns the user, group or client
func processRelation(tree *rts.SubjectTree) (user *User, group *Group, client *OAuth2Client) {
	switch tree.Tuple.Subject.GetSet().Namespace {
	case consts.UserNamespace.String():
		user = &User{
			ID: tree.Tuple.Subject.GetSet().Object,
		}
	case consts.GroupNamespace.String():
		group = &Group{
			Name: tree.Tuple.Subject.GetSet().Object,
		}
	case consts.OAuth2ClientNamespace.String():
		client = &OAuth2Client{
			ClientID: &tree.Tuple.Subject.GetSet().Object,
		}
	}
	return user, group, client
}

// function that returns a list of user IDs from a []*UserInput
func getUserIdsFromUserInputs(ctx context.Context, users []*UserInput) ([]string, error) {
	clients := common.GetContext(ctx)
	log := clients.Log.WithName("GetUserIdsFromUserInputs")

	ctx, span := clients.Tracer.Start(ctx, "GetUserIdsFromUserInputs")
	defer span.End()

	var ids []string

	for _, inputUser := range users {
		if inputUser.ID != nil {
			if *inputUser.ID != "" {
				ids = append(ids, *inputUser.ID)
			}
		} else if inputUser.Email != nil {
			if *inputUser.Email != "" {
				user, err := helpers.GetUserFromEmail(ctx, *inputUser.Email)
				if err != nil {
					log.Error(err, "failed to get user from email", "Email", *inputUser.Email)
					continue
				}
				ids = append(ids, user.Id)
			}
		}
	}
	return ids, nil
}
