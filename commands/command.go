package commands

import (
	"github.com/slack-go/slack"
)

// Command is the interface for all slash commands
type Command interface {
	Execute(cmd slack.SlashCommand) error
}
