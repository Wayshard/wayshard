//go:build windows

package storage

import (
	"golang.org/x/sys/windows"
)

func Disk(path string) (DiskStatus, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return DiskStatus{Path: path}, err
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree); err != nil {
		return DiskStatus{Path: path}, err
	}
	d := DiskStatus{Path: path, Total: total, Free: free, Used: total - free, Threshold: defaultLowBytes}
	d.Low = free < defaultLowBytes
	d.Critical = free < defaultCriticalBytes
	return d, nil
}
