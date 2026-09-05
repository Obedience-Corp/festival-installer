package launch

import (
	"context"

	"github.com/Obedience-Corp/festival-installer/internal/app"
)

// Resolve finds the absolute path to tool. The rule lives in the app package,
// because the hub itself has to resolve suite tools to answer questions about
// setup, and app cannot import launch.
func Resolve(ctx context.Context, tool string) (string, error) {
	return app.ResolveTool(ctx, tool)
}
