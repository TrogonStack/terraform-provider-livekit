package provider

import (
	"errors"

	"github.com/twitchtv/twirp"
)

func isNotFound(err error) bool {
	var twerr twirp.Error
	if errors.As(err, &twerr) {
		return twerr.Code() == twirp.NotFound
	}
	return false
}
