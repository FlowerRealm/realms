package admin

import (
	"net/url"
	"strings"
)

func isChannelGroupsUpdateForm(form url.Values) bool {
	if form == nil {
		return false
	}
	if strings.TrimSpace(form.Get("_intent")) == "update_channel_groups" {
		return true
	}
	_, ok := form["groups"]
	return ok
}
