package cmd

import "testing"

func TestNotifTarget(t *testing.T) {
	cases := []struct {
		ch   map[string]interface{}
		want string
	}{
		{map[string]interface{}{"type": "smtp", "host": "smtp.test", "port": "587", "from": "a@x.it", "to": []interface{}{"b@x.it", "c@x.it"}}, "smtp.test:587  a@x.it → b@x.it, c@x.it"},
		{map[string]interface{}{"type": "telegram", "chat_id": "-100"}, "chat -100"},
		{map[string]interface{}{"type": "webhook", "url": "https://h.test/***", "secret_set": true}, "https://h.test/***  signed"},
		{map[string]interface{}{"type": "ntfy", "url": "https://ntfy.sh/***", "priority": "high"}, "https://ntfy.sh/***  priority=high"},
		{map[string]interface{}{"type": "slack", "url": "https://hooks.slack.com/services/***"}, "https://hooks.slack.com/services/***"},
		{map[string]interface{}{"type": "resend", "from": "a@x.it", "to": "b@x.it"}, "a@x.it → b@x.it"},
	}
	for _, c := range cases {
		if got := notifTarget(c.ch); got != c.want {
			t.Errorf("%v: got %q want %q", c.ch["type"], got, c.want)
		}
	}
}

func TestNotifGroupTriggers(t *testing.T) {
	order, groups := notifGroupTriggers([]interface{}{
		map[string]interface{}{"id": "deploy_fail", "category": "Deploy"},
		map[string]interface{}{"id": "ssh_login", "category": "Security"},
		map[string]interface{}{"id": "deploy_success", "category": "Deploy"},
	})
	if len(order) != 2 || order[0] != "Deploy" || order[1] != "Security" {
		t.Fatalf("order = %v", order)
	}
	if len(groups["Deploy"]) != 2 || groups["Deploy"][1]["id"] != "deploy_success" {
		t.Fatalf("groups = %v", groups)
	}
}

func TestNotifBody(t *testing.T) {
	c := notifUpdateCmd
	_ = c.Flags().Set("chat-id", "-2")
	_ = c.Flags().Set("tls", "false")
	b := notifBody(c)
	if b["chat_id"] != "-2" || b["tls"] != false || len(b) != 2 {
		t.Fatalf("body = %v", b)
	}
}
