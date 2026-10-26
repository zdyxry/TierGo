package easytier

import (
	"context"
	"fmt"

	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// LoggerRpc updates both Core log filters without restarting the process.
// It is process-wide, including other instances in the same service daemon.
func SetRuntimeLogLevel(ctx context.Context, address, level string) error {
	var numeric uint64
	switch level {
	case "off":
		numeric = 0
	case "error":
		numeric = 1
	case "warn":
		numeric = 2
	case "info":
		numeric = 3
	case "debug":
		numeric = 4
	case "trace":
		numeric = 5
	default:
		return fmt.Errorf(locale.T("无效日志级别 %q"), level)
	}
	_, err := managementRPC(ctx, address, "LoggerRpc", 1, wireInt(nil, 1, numeric))
	return err
}
