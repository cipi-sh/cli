package cmd

import (
	"fmt"
	"regexp"
	"strings"
)

// A database made with a custom or a Node app (apps create --db NAME, Cipi
// 5.18.0+): the name the server takes, checked here before the request.
var appDBNameRE = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)

// appCreateDB returns the database name to send with an app create, or ""
// for none. A Laravel app is created with its own database and takes no other.
func appCreateDB(name string, laravel bool) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil
	}
	if laravel {
		return "", fmt.Errorf("--db is for custom and Node apps: a Laravel app is created with its own database, named like the app (one more, afterwards: cipi-cli db create --name %s --app <app>)", name)
	}
	if !appDBNameRE.MatchString(name) {
		return "", fmt.Errorf("--db %q is not a database name: lowercase letters, digits and _, starting with a letter (2 to 64 characters)", name)
	}
	return name, nil
}
