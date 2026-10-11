# Antimatter Todo Plugin

Todo lists for [Antimatter](https://github.com/antimatterchat/antimatter): a shared list in every channel, your own
list, and todos you send to people and receive from them, with a daily reminder.

<img src="assets/screenshot.png" width="440" alt="The Todo panel: the shared todo list of a channel, with an assigned todo and a done one" />

## Using it

Open **Todo** from the app bar (or the channel header). Its panel has two views:

- **This channel**: the channel's todo list, shared by its members. Everyone who can post in the channel adds
  todos, checks them off, edits them, assigns them to a member and moves them to the top; their author and the
  channel's admins delete them. Done todos are folded under the open ones. People who can only read the
  channel (a read-only or archived one) see the list without changing it.
- **Mine**: your own todos, the channel todos assigned to you, the todos people sent you (accept, mark as done
  or decline them), and the ones you sent (remind the person or cancel).

A message's **Add to todo** starts a todo with its text, linked to the message, in the channel's list or your
own. When someone assigns you a channel todo or sends you one, the Todo bot tells you, and its message lets you
answer. Once a day, the bot reminds you of your todos and the ones assigned to you.

The `/todo` command still works: `/todo add`, `/todo list [my|in|out]`, `/todo pop`, `/todo send @someone`,
`/todo settings summary [on|off]` and `/todo settings allow_incoming_task_requests [on|off]`.

## Installation

1. Download the latest release from https://github.com/antimatterchat/antimatter-plugin-todo/releases/latest.
2. Upload it in **System Console > Plugins > Plugin Management**, then enable it.

The plugin keeps its upstream ID, `com.mattermost.plugin-todo`, so the lists of an existing installation carry
over.

## Development

```sh
make dist          # builds the server for every platform, the web app, and the bundle in dist/
make test          # Go and web app tests
make check-style   # golangci-lint, eslint and the type checker
```

Set `MM_SERVICESETTINGS_ENABLEDEVELOPER=1` to build the server for your platform only.

## Thanks

This plugin started as [mattermost-plugin-todo](https://github.com/mattermost/mattermost-plugin-todo), a community
plugin of the Mattermost ecosystem. Many thanks to the Mattermost community, and to everyone who contributed to
it, for the lists, the requests between people, the daily reminders and the `/todo` command this plugin builds
on.

## License

Apache License 2.0, see [LICENSE](LICENSE).
