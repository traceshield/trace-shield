package resolvers

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.34

import (
	"context"

	"github.com/pluralsh/trace-shield-controller/api/observability/v1alpha1"
	"github.com/pluralsh/trace-shield/graph/generated"
	"github.com/pluralsh/trace-shield/graph/model"
)

// SourceLabels is the resolver for the sourceLabels field.
func (r *relabelConfigResolver) SourceLabels(ctx context.Context, obj *v1alpha1.RelabelConfig) ([]*string, error) {
	output := make([]*string, len(obj.SourceLabels))
	for i, label := range obj.SourceLabels {
		s := string(*label)
		output[i] = &s
	}
	return output, nil
}

// Action is the resolver for the action field.
func (r *relabelConfigResolver) Action(ctx context.Context, obj *v1alpha1.RelabelConfig) (*model.RelabelAction, error) {
	if obj.Action == nil {
		return nil, nil
	} else {
		output := model.RelabelAction(*obj.Action)
		return &output, nil
	}
}

// RelabelConfig returns generated.RelabelConfigResolver implementation.
func (r *Resolver) RelabelConfig() generated.RelabelConfigResolver { return &relabelConfigResolver{r} }

type relabelConfigResolver struct{ *Resolver }