package logrotate

import (
	"compress/gzip"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"math/rand"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/pkg/errors"
)

type RotateFile struct {
	fileName string
	dirName string
	baseName string
	ext string
	maxAge time.Duration
	maxSize int64
	maxBackups int
	timeZone *time.Location
	curSize int64
	file *os.File
	lock *sync.Mutex
	start *time.Time
	nextStart time.Time
	closed bool
}

func Open(fn string, maxAge time.Duration, maxSize int64, count int) (*RotateFile, error) {
	dn, bn := filepath.Split(fn)
	ext := filepath.Ext(bn)
	bn = bn[:len(bn) - len(ext)]
	l := &RotateFile{
		fileName: fn,
		dirName: dn,
		baseName: bn,
		ext: ext,
		maxAge: maxAge,
		maxSize: maxSize,
		maxBackups: count,
		timeZone: time.Local,
		curSize: 0,
		lock: &sync.Mutex{},
		file: nil,
		start: nil,
		closed: false,
	}
	if fn == "" {
		l.file = os.Stderr
	}
	/*
	err := l.open()
	if err != nil {
		return nil, err
	}
	*/
	return l, nil
}

func (l *RotateFile) Name() string {
	f := l.file
	if f != nil {
		return f.Name()
	}
	return l.fileName
}

/*
func (l *RotateFile) SetName(fn string) error {
	l.lock.Lock()
	defer l.Unlock()
	dn, bn := filepath.Split(fn)
	ext := filepath.Ext(bn)
	bn = bn[:len(bn) - len(ext)]
	l.close()
	l.fileName = fn
	l.dirName = dn
	l.baseName = bn
	l.ext = ext
	return l.open()
}
*/

func (l *RotateFile) MaxSize() int64 {
	return l.maxSize
}

func (l *RotateFile) SetMaxSize(size int64) {
	l.lock.Lock()
	defer l.lock.Unlock()
	l.maxSize = size
}

func (l *RotateFile) MaxAge() time.Duration {
	return l.maxAge
}

func (l *RotateFile) SetMaxAge(age time.Duration) {
	l.lock.Lock()
	defer l.lock.Unlock()
	l.maxAge = age
	l.nextStart = l.nextRotate()
}

func (l *RotateFile) MaxBackups() int {
	return l.maxBackups
}

func (l *RotateFile) SetMaxBackups(count int) {
	l.lock.Lock()
	defer l.lock.Unlock()
	l.maxBackups = count
}

func (l *RotateFile) TimeZone() *time.Location {
	return l.timeZone
}

func (l *RotateFile) SetTimeZone(tz *time.Location) {
	l.lock.Lock()
	defer l.lock.Unlock()
	l.timeZone = tz
	if l.start != nil {
		t := l.start.In(tz)
		l.start = &t
		l.nextStart = l.nextRotate()
	}
}

func (l *RotateFile) Close() error {
	l.lock.Lock()
	defer l.lock.Unlock()
	return l.close()
}

func (l *RotateFile) close() error {
	if l.closed {
		return nil
	}
	l.closed = true
	if l.fileName != "" && l.file != nil {
		err := l.file.Close()
		if err != nil {
			return err
		}
	}
	l.file = nil
	return nil
}

func (l *RotateFile) Write(data []byte) (int, error) {
	l.lock.Lock()
	defer l.lock.Unlock()
	if l.closed {
		if l.file == nil {
			return -1, errors.New("write: no file open")
		}
		return -1, errors.Errorf("write %s: file already closed", l.file.Name())
	}
	if l.file == nil {
		err := l.open()
		if err != nil {
			return -1, err
		}
	}
	now := time.Now().In(l.timeZone)
	size := int64(len(data))
	if l.needsRotate(now, size) {
		err := l.rotate()
		if err != nil {
			return -1, err
		}
	}
	n, err := l.file.Write(data)
	if n > 0 {
		l.curSize += int64(n)
	}
	if err != nil {
		return n, err
	}
	l.file.Sync()
	return n, nil
}

func (l *RotateFile) open() error {
	if l.fileName == "" {
		l.file = os.Stderr
		return nil
	}
	var err error
	l.file, err = os.OpenFile(l.fileName, os.O_WRONLY | os.O_APPEND | os.O_CREATE, 0644)
	if err != nil {
		return errors.Wrapf(err, "can't open log file %s", l.fileName)
	}
	st, err := os.Stat(l.fileName)
	if err == nil {
		l.curSize = st.Size()
	} else {
		l.curSize = 0
	}
	return nil
}

func (l *RotateFile) needsRotate(now time.Time, size int64) bool {
	if l.start == nil {
		l.start = &now
		l.nextStart = l.nextRotate()
	}
	if !now.Before(l.nextStart) {
		return true
	}
	if l.maxSize > 0 && l.curSize + size > l.maxSize {
		return true
	}
	return false
}

func (l *RotateFile) nextRotate() time.Time {
	if l.start == nil {
		return time.Time{}
	}
	now := l.start.In(l.timeZone)
	var next time.Time
	month := time.Duration(30 * 24 * time.Hour)
	week := time.Duration(7 * 24 * time.Hour)
	day := time.Duration(24 * time.Hour)
	hour := time.Hour
	dur := l.maxAge
	if dur < time.Minute {
		dur = day
	}
	if dur % month == 0 {
		y := now.Year()
		mn := int(now.Month()) + int(dur / month)
		for mn > 12 {
			mn -= 12
			y += 1
		}
		next = time.Date(y, time.Month(mn), 1, 0, 0, 0, 0, l.timeZone)
	} else if dur % week == 0 {
		wn := int(dur / week)
		next = time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, l.timeZone)
		next = next.AddDate(0, 0, wn * 7 - int(next.Weekday()))
		next = time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, l.timeZone)
	} else if dur % day == 0 {
		next = time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, l.timeZone)
		next = next.AddDate(0, 0, int(dur / day))
		next = time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, l.timeZone)
	} else if dur % hour == 0 {
		hn := int(dur / hour)
		hr := ((now.Hour() + hn) / hn) * hn
		next = time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, l.timeZone)
		for hr >= 24 {
			next = next.AddDate(0, 0, 1)
			hr -= 24
		}
		next = time.Date(next.Year(), next.Month(), next.Day(), hr, 0, 0, 0, l.timeZone)
	} else {
		mn := int(dur / time.Minute)
		next = now.Add(time.Duration(mn) * time.Minute)
		next = time.Date(next.Year(), next.Month(), next.Day(), next.Hour(), mn * (next.Minute() / mn), 0, 0, l.timeZone)
	}
	return next
}

func (l *RotateFile) timestampFormat() string {
	if l.maxAge >= time.Duration(30 * 24 * time.Hour) {
		return "200601"
	}
	if l.maxAge >= time.Duration(24 * time.Hour) {
		return "20060102"
	}
	if l.maxAge >= time.Hour {
		return "20060102T15"
	}
	return "20060102T1504"
}

func (l *RotateFile) Rotate() error {
	l.lock.Lock()
	defer l.lock.Unlock()
	return l.rotate()
}

func (l *RotateFile) rotate() error {
	fn, ts, err := l.rotateOnly()
	if err != nil {
		return err
	}
	if fn != "" {
		go l.compressAndCleanup(fn, ts)
	}
	return nil
}

func (l *RotateFile) rotateOnly() (string, string, error) {
	if l.closed || l.file == nil {
		return "", "", errors.New("can't rotate closed log file")
	}
	if l.fileName == "" {
		return "", "", nil
	}
	l.file.Write([]byte("rotating " + l.fileName + "\n"))
	l.file.Close()
	l.file = nil
	rnd := make([]byte, 8)
	rand.Read(rnd)
	start := *l.start
	ts := start.Format(l.timestampFormat())
	fn := filepath.Join(l.dirName, fmt.Sprintf("%s-%s_x%s%s", l.baseName, ts, hex.EncodeToString(rnd), l.ext))
	_, err := os.Stat(fn)
	if err == nil {
		return "", "", errors.Errorf("rotated log file %s already exists", fn)
	}
	if !os.IsNotExist(err) {
		return "", "", errors.Wrap(err, "can't stat rotation file " + fn)
	}
	err = os.Rename(l.fileName, fn)
	if err != nil {
		return "", "", errors.Wrapf(err, "can't rotate log file %s to %s", l.fileName, fn)
	}
	l.start = nil
	return fn, ts, nil
}

func (l *RotateFile) compressAndCleanup(fn string, ts string) error {
	_, err := l.compress(fn, ts)
	if err != nil {
		return err
	}
	return l.cleanup()
}

func (l *RotateFile) compress(fn string, ts string) (string, error) {
	r, err := os.Open(fn)
	if err != nil {
		return "", errors.Wrapf(err, "can't open uncompresed log file %s", fn)
	}
	defer r.Close()
	var gzfn string
	var w io.WriteCloser
	for i := 0; i < 1000; i++ {
		gzfn = filepath.Join(l.dirName, fmt.Sprintf("%s-%s_%03d%s.gz", l.baseName, ts, i, l.ext))
		_, err = os.Stat(gzfn)
		if err != nil && os.IsNotExist(err) {
			w, err = os.OpenFile(gzfn, os.O_CREATE | os.O_WRONLY | os.O_EXCL, 0644)
			if err == nil {
				break
			}
			w = nil
		}
	}
	if w == nil {
		return "", errors.Errorf("backup overrun, more than 1000 backup files for %s", gzfn)
	}
	gzw := gzip.NewWriter(w)
	_, err = io.Copy(gzw, r)
	if err != nil {
		return gzfn, errors.Wrapf(err, "error gzipping log file %s", gzfn)
	}
	err = gzw.Close()
	if err != nil {
		return gzfn, errors.Wrapf(err, "error gzipping log file %s (flush)", gzfn)
	}
	err = w.Close()
	if err != nil {
		return gzfn, errors.Wrapf(err, "error gzipping log file %s (close)", gzfn)
	}
	err = os.Remove(fn)
	if err != nil {
		return gzfn, errors.Wrapf(err, "error removing uncompressed log file %s", fn)
	}
	return gzfn, nil
}

func (l *RotateFile) cleanup() error {
	if l.maxBackups <= 0 {
		return nil
	}
	pat := filepath.Join(l.dirName, fmt.Sprintf("%s-*_[0-9][0-9][0-9]%s.gz", l.baseName, l.ext))
	fns, err := filepath.Glob(pat)
	if err != nil {
		return errors.Wrapf(err, "error looking for old backup files %s", pat)
	}
	if len(fns) <= l.maxBackups {
		return nil
	}
	sort.Strings(fns)
	fmt.Println("keep", l.maxBackups, "of", fns)
	for _, dfn := range fns[:len(fns) - l.maxBackups] {
		fmt.Println("remove", dfn)
		os.Remove(dfn)
	}
	return nil
}
