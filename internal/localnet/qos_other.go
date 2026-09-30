//go:build !windows

package localnet

import (
	"context"
	"fmt"
)

func QoS(ctx context.Context, dir, action string, rules []Rule) (string, error) {
	return "", fmt.Errorf("ограничения по .exe используют Windows NetQos; на Linux доступны прокси, измерения и автозапуск")
}
