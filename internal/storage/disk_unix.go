//go:build unix

package storage

import "syscall"

func Disk(path string) (DiskStatus, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return DiskStatus{Path: path}, err
	}
	total := uint64(st.Blocks) * uint64(st.Bsize)
	free := uint64(st.Bavail) * uint64(st.Bsize)
	d := DiskStatus{Path: path, Total: total, Free: free, Used: total - free, Threshold: defaultLowBytes}
	d.Low = free < defaultLowBytes
	d.Critical = free < defaultCriticalBytes
	return d, nil
}
