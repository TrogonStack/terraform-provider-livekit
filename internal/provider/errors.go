package provider

import (
	"errors"
	"strings"

	"github.com/twitchtv/twirp"
)

// notFoundInternalMessages are substrings of twirp.Internal error messages that the
// CloudAgent API uses in place of a proper twirp.NotFound when the agent behind a
// lookup no longer exists.
var notFoundInternalMessages = []string{
	"failed to get agent",
	"could not be found",
}

func isNotFound(err error) bool {
	var twerr twirp.Error
	if !errors.As(err, &twerr) {
		return false
	}
	if twerr.Code() == twirp.NotFound {
		return true
	}
	if twerr.Code() != twirp.Internal {
		return false
	}
	msg := twerr.Msg()
	for _, m := range notFoundInternalMessages {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return false
}
