package provider

import (
	"errors"
	"fmt"
	"testing"

	"github.com/twitchtv/twirp"
)

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "twirp not found",
			err:  twirp.NewError(twirp.NotFound, "agent not found"),
			want: true,
		},
		{
			name: "wrapped twirp not found",
			err:  fmt.Errorf("wrapped: %w", twirp.NewError(twirp.NotFound, "agent not found")),
			want: true,
		},
		{
			name: "internal failed to get agent",
			err:  twirp.NewError(twirp.Internal, "failed to get agent"),
			want: true,
		},
		{
			name: "internal agent could not be found",
			err:  twirp.NewError(twirp.Internal, "The agent could not be found. Please check the agent ID and try again."),
			want: true,
		},
		{
			name: "internal unrelated message",
			err:  twirp.NewError(twirp.Internal, "something else went wrong"),
			want: false,
		},
		{
			name: "other twirp code",
			err:  twirp.NewError(twirp.InvalidArgument, "bad request"),
			want: false,
		},
		{
			name: "non twirp error",
			err:  errors.New("boom"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNotFound(tt.err); got != tt.want {
				t.Errorf("isNotFound(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
