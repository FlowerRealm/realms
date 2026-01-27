package admin

import (
	"net/url"
	"testing"
)

func TestIsChannelGroupsUpdateForm(t *testing.T) {
	if isChannelGroupsUpdateForm(nil) {
		t.Fatalf("expected false for nil form")
	}

	if !isChannelGroupsUpdateForm(url.Values{"groups": []string{"default"}}) {
		t.Fatalf("expected true when groups key is present")
	}

	if !isChannelGroupsUpdateForm(url.Values{"_intent": []string{"update_channel_groups"}}) {
		t.Fatalf("expected true when intent is update_channel_groups")
	}

	if isChannelGroupsUpdateForm(url.Values{"return_to": []string{"/admin/channels?open_channel_settings=1#groups"}}) {
		t.Fatalf("expected false when only return_to is present")
	}

	if isChannelGroupsUpdateForm(url.Values{"channel_id": []string{"1"}}) {
		t.Fatalf("expected false when only channel_id is present")
	}

	if isChannelGroupsUpdateForm(url.Values{"base_url": []string{"https://api.openai.com"}}) {
		t.Fatalf("expected false when only base_url is present")
	}
}
