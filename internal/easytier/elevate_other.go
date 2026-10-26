//go:build !linux && !darwin && !windows

package easytier

import "errors"

// Other platforms have no TUN support here, so there is nothing to elevate.
func IsElevated() bool { return false }

func RelaunchElevated(string) error {
	return errors.New(locale.T("当前平台不支持虚拟网卡"))
}
