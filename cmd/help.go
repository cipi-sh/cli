package cmd

// Shared help fragments used across command Long/Example text.

const multiServerTip = `Multi-server: each profile is one Cipi server (endpoint + token).
Prefix any command with a profile name to target that server:

  cipi-cli prod apps list
  cipi-cli staging deploy myapp
  cipi-cli apps list                 # uses the default profile`

const profilesHelp = `A profile is one Cipi server (API endpoint + token).

Add a server:
  cipi-cli configure --profile prod
  cipi-cli profiles add staging
  cipi-cli api token add prod

List servers:
  cipi-cli profiles
  cipi-cli servers                   # alias of profiles

Set the default server (used when you omit the profile prefix):
  cipi-cli profiles use prod

Delete a server:
  cipi-cli profiles delete staging

Run a command against a server:
  cipi-cli prod apps list            # explicit profile
  cipi-cli apps list                 # uses the default profile

Config file: ~/.cipi/config.json`
