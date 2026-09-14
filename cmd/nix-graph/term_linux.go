//go:build linux

package main

import (
	"os"
	"syscall"
	"unsafe"
)

// readKeys polls stdin with epoll and forwards chunks of bytes to ch;
// it returns when stop is closed.
func readKeys(ch chan<- []byte, stop <-chan struct{}) {
	defer close(ch)
	fd := int(os.Stdin.Fd())
	epfd, err := syscall.EpollCreate1(syscall.EPOLL_CLOEXEC)
	if err != nil {
		return
	}
	defer syscall.Close(epfd)
	ev := syscall.EpollEvent{Events: syscall.EPOLLIN}
	if err := syscall.EpollCtl(epfd, syscall.EPOLL_CTL_ADD, fd, &ev); err != nil {
		return
	}
	buf := make([]byte, 64)
	events := make([]syscall.EpollEvent, 1)
	for {
		n, err := syscall.EpollWait(epfd, events, 100)
		if err != nil && err != syscall.EINTR {
			return
		}
		select {
		case <-stop:
			return
		default:
		}
		if n == 0 {
			continue
		}
		nn, err := syscall.Read(fd, buf)
		if nn > 0 {
			b := make([]byte, nn)
			copy(b, buf[:nn])
			select {
			case ch <- b:
			case <-stop:
				return
			}
		}
		if err != nil {
			if err == syscall.EINTR || err == syscall.EAGAIN {
				continue
			}
			return
		}
	}
}

func makeRaw(fd int) (*syscall.Termios, error) {
	var old syscall.Termios
	if err := ioctl(fd, syscall.TCGETS, unsafe.Pointer(&old)); err != nil {
		return nil, err
	}
	raw := old
	raw.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP |
		syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.IEXTEN
	raw.Cflag &^= syscall.CSIZE | syscall.PARENB
	raw.Cflag |= syscall.CS8
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0
	if err := ioctl(fd, syscall.TCSETS, unsafe.Pointer(&raw)); err != nil {
		return nil, err
	}
	return &old, nil
}

func restoreTerm(fd int, old *syscall.Termios) {
	if old != nil {
		_ = ioctl(fd, syscall.TCSETS, unsafe.Pointer(old))
	}
}
