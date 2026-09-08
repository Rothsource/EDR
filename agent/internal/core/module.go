package core

import "context"

// Every capability (email, process, network, file, response) implements this.
// Run should watch ctx.Done() and return promptly when it fires — that's
// what makes shutdown "graceful" instead of "killed mid-request."
type Module interface {
	Name() string
	Run(ctx context.Context) error
}
