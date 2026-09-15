//go:build darwin

package main

import (
	"os"
	"syscall"
	"time"
	"unsafe"
)

// the syscall package does not expose VMIN/VTIME on darwin; these are
// their slots in the Termios Cc array 
const (
	ccMin  = 16
	ccTime = 17
)

// readKeys polls stdin with kqueue and forwards chunks of bytes to ch;
// it returns when stop is closed.
func readKeys(ch chan<- []byte, stop <-chan struct{}) {
	defer close(ch)
	fd := int(os.Stdin.Fd())
	kq, err := syscall.Kqueue()
	if err != nil {
		return
	}
	defer syscall.Close(kq)
	add := syscall.Kevent_t{
		Ident:  uint64(fd),
		Filter: syscall.EVFILT_READ,
		Flags:  syscall.EV_ADD,
	}
	if _, err := syscall.Kevent(kq, []syscall.Kevent_t{add}, nil, nil); err != nil {
		return
	}
	buf := make([]byte, 64)
	events := make([]syscall.Kevent_t, 1)
	wait := syscall.NsecToTimespec(int64(100 * time.Millisecond))
	for {
		n, err := syscall.Kevent(kq, nil, events, &wait)
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
	if err := ioctl(fd, syscall.TIOCGETA, unsafe.Pointer(&old)); err != nil {
		return nil, err
	}
	raw := old
	raw.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP |
		syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.IEXTEN
	raw.Cflag &^= syscall.CSIZE | syscall.PARENB
	raw.Cflag |= syscall.CS8
	raw.Cc[ccMin] = 1
	raw.Cc[ccTime] = 0
	if err := ioctl(fd, syscall.TIOCSETA, unsafe.Pointer(&raw)); err != nil {
		return nil, err
	}
	return &old, nil
}

func restoreTerm(fd int, old *syscall.Termios) {
	if old != nil {
		_ = ioctl(fd, syscall.TIOCSETA, unsafe.Pointer(old))
	}
}
