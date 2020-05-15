package logrotate

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	. "gopkg.in/check.v1"
)

func tempFile(ext string) string {
	var randBytes = make([]byte, 16)
	rand.Read(randBytes)
	return filepath.Join(os.TempDir(), hex.EncodeToString(randBytes) + ext)
}

func Test(t *testing.T) { TestingT(t) }
type LogRotateSuite struct {}
var _ = Suite(&LogRotateSuite{})

func (a *LogRotateSuite) TestOpen(c *C) {
	fn := tempFile(".log")
	defer os.Remove(fn)
	rf, err := Open(fn, time.Minute, -1, 1)
	c.Check(err, IsNil)
	c.Check(rf, NotNil)
	st, err := os.Stat(fn)
	c.Check(err, NotNil)
	c.Check(st, IsNil)
	rf.Write([]byte("testing\n"))
	st, err = os.Stat(fn)
	c.Check(err, IsNil)
	c.Check(st, NotNil)
}

func (a *LogRotateSuite) TestWrite(c *C) {
	fn := tempFile(".log")
	defer os.Remove(fn)
	rf, err := Open(fn, time.Minute, -1, 1)
	c.Check(err, IsNil)
	c.Check(rf, NotNil)
	c.Check(rf.start, IsNil)
	n, err := rf.Write([]byte("abcd\n"))
	c.Check(err, IsNil)
	c.Check(n, Equals, 5)
	c.Check(rf.start, NotNil)
	st, err := os.Stat(fn)
	c.Check(err, IsNil)
	c.Check(st, NotNil)
	c.Check(st.Size(), Equals, int64(5))
}

func (a *LogRotateSuite) TestOpenStderr(c *C) {
	fn := tempFile(".stderr")
	fp, err := os.Create(fn)
	c.Assert(err, IsNil)
	orig := os.Stderr
	os.Stderr = fp
	defer os.Remove(fn)
	rf, err := Open("", time.Minute, -1, 1)
	os.Stderr = orig
	c.Check(err, IsNil)
	c.Check(rf, NotNil)
	n, err := rf.Write([]byte("abcd\n"))
	c.Check(err, IsNil)
	c.Check(n, Equals, 5)
	st, err := os.Stat(fn)
	c.Check(err, IsNil)
	c.Check(st, NotNil)
	c.Check(st.Size(), Equals, int64(5))
}

func (a *LogRotateSuite) TestName(c *C) {
	rf, _ := Open("", 30 * 24 * time.Hour, -1, 1)
	c.Check(rf.Name(), Equals, "/dev/stderr")
	fn := tempFile(".log")
	defer os.Remove(fn)
	rf, _ = Open(fn, 30 * 24 * time.Hour, -1, 1)
	c.Check(rf.Name(), Equals, fn)
	rf.Write([]byte("testing\n"))
	rf.fileName = tempFile(".xlog")
	c.Check(rf.Name(), Equals, fn)
}

func (a *LogRotateSuite) TestConfig(c *C) {
	fn := tempFile(".log")
	defer os.Remove(fn)
	rf, _ := Open(fn, 30 * 24 * time.Hour, 1024, 4)
	c.Check(rf.Name(), Equals, fn)
	c.Check(rf.MaxAge(), Equals, 30 * 24 * time.Hour)
	c.Check(rf.MaxSize(), Equals, int64(1024))
	c.Check(rf.MaxBackups(), Equals, 4)
	c.Check(rf.TimeZone(), Equals, time.Local)
	now := time.Now()
	rf.start = &now
	rf.nextStart = rf.nextRotate()
	y := now.Year()
	m := now.Month()
	if m == time.December {
		m = time.January
		y += 1
	} else {
		m += 1
	}
	eom := time.Date(y, m, 1, 0, 0, 0, 0, time.Local)
	c.Check(rf.nextStart.Format("2006-01-02 15:04:05"), Equals, eom.Format("2006-01-02 15:04:05"))
	c.Check(rf.nextStart.Unix(), Equals, eom.Unix())
	eow := now.AddDate(0, 0, 7 - int(now.Weekday()))
	eow = time.Date(eow.Year(), eow.Month(), eow.Day(), 0, 0, 0, 0, time.Local)
	rf.SetMaxAge(7 * 24 * time.Hour)
	c.Check(rf.MaxAge(), Equals, 7 * 24 * time.Hour)
	c.Check(rf.nextStart.Unix(), Equals, eow.Unix())
	rf.SetMaxSize(2048)
	c.Check(rf.MaxSize(), Equals, int64(2048))
	rf.SetMaxBackups(3)
	c.Check(rf.MaxBackups(), Equals, 3)
	utcnow := now.In(time.UTC)
	utceow := utcnow.AddDate(0, 0, 7 - int(utcnow.Weekday()))
	utceow = time.Date(eow.Year(), eow.Month(), eow.Day(), 0, 0, 0, 0, time.UTC)
	rf.SetTimeZone(time.UTC)
	c.Check(rf.TimeZone(), Equals, time.UTC)
	c.Check(rf.nextStart.Unix(), Equals, utceow.Unix())
}

func (a *LogRotateSuite) TestNextRotate(c *C) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	c.Assert(err, IsNil)
	orig := time.Local
	time.Local = loc
	defer func() { time.Local = orig }()
	fn := tempFile(".log")
	defer os.Remove(fn)
	rf, err := Open(fn, 30 * 24 * time.Hour, -1, 1)
	now := time.Date(2019, time.December, 30, 22, 27, 9, 12345, time.UTC)
	rf.start = &now
	next := rf.nextRotate().In(time.UTC).Format("2006-01-02 15:04:05")
	c.Check(next, Equals, "2020-01-01 08:00:00")
	rf, err = Open(fn, 7 * 24 * time.Hour, -1, 1)
	rf.start = &now
	next = rf.nextRotate().In(time.UTC).Format("2006-01-02 15:04:05")
	c.Check(next, Equals, "2020-01-05 08:00:00")
	rf, err = Open(fn, 24 * time.Hour, -1, 1)
	rf.start = &now
	next = rf.nextRotate().In(time.UTC).Format("2006-01-02 15:04:05")
	c.Check(next, Equals, "2019-12-31 08:00:00")
	rf, err = Open(fn, time.Hour, -1, 1)
	rf.start = &now
	next = rf.nextRotate().In(time.Local).Format("2006-01-02 15:04:05")
	c.Check(next, Equals, "2019-12-30 15:00:00")
	rf, err = Open(fn, 12 * time.Hour, -1, 1)
	rf.start = &now
	next = rf.nextRotate().In(time.Local).Format("2006-01-02 15:04:05")
	c.Check(next, Equals, "2019-12-31 00:00:00")
	rf, err = Open(fn, 20 * time.Minute, -1, 1)
	rf.start = &now
	next = rf.nextRotate().In(time.Local).Format("2006-01-02 15:04:05")
	c.Check(next, Equals, "2019-12-30 14:40:00")
	rf, err = Open(fn, 20 * time.Second, -1, 1)
	rf.start = &now
	next = rf.nextRotate().In(time.Local).Format("2006-01-02 15:04:05")
	c.Check(next, Equals, "2019-12-31 00:00:00")
}

func (a *LogRotateSuite) TestClose(c *C) {
	fn := tempFile(".log")
	defer os.Remove(fn)
	rf, _ := Open(fn, time.Hour, -1, 1)
	c.Check(rf.closed, Equals, false)
	err := rf.Close()
	c.Check(err, IsNil)
	c.Check(rf.closed, Equals, true)
	c.Check(rf.file, IsNil)
	rf, _ = Open(fn, time.Hour, -1, 1)
	rf.Write([]byte("abcd\n"))
	err = rf.Close()
	c.Check(err, IsNil)
	c.Check(rf.closed, Equals, true)
	c.Check(rf.file, IsNil)
	err = rf.Close()
	c.Check(err, IsNil)
	c.Check(rf.closed, Equals, true)
	c.Check(rf.file, IsNil)
	stderr := tempFile(".stderr")
	defer os.Remove(stderr)
	fp, err := os.Create(fn)
	c.Assert(err, IsNil)
	orig := os.Stderr
	os.Stderr = fp
	rf, _ = Open("", time.Hour, -1, 1)
	os.Stderr = orig
	rf.Write([]byte("line1\n"))
	err = rf.Close()
	c.Check(err, IsNil)
	c.Check(rf.closed, Equals, true)
	c.Check(rf.file, IsNil)
	_, err = rf.Write([]byte("line2\n"))
	c.Check(err, NotNil)
	c.Check(rf.closed, Equals, true)
	c.Check(rf.file, IsNil)
	_, err = fp.Write([]byte("line3\n"))
	c.Check(err, IsNil)
	fp.Close()
}

func (a *LogRotateSuite) TestNeedsRotate(c *C) {
	fn := tempFile(".log")
	defer os.Remove(fn)
	rf, _ := Open(fn, time.Hour, 1024, 2)
	now := time.Now()
	now = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), 30, 0, now.Location())
	earlier := now.Add(-time.Hour)
	c.Check(rf.needsRotate(earlier, 100), Equals, false)
	c.Check(rf.needsRotate(now, 100), Equals, true)
	later := now.Add(time.Second)
	rf.start = nil
	c.Check(rf.needsRotate(later, 100), Equals, false)
	rf.curSize += 1000
	c.Check(rf.needsRotate(later, 100), Equals, true)
}

func (a *LogRotateSuite) TestTimestampFormat(c *C) {
	fn := tempFile(".log")
	defer os.Remove(fn)
	rf, _ := Open(fn, 15 * time.Minute, 1024, 2)
	c.Check(rf.timestampFormat(), Equals, "20060102T1504")
	rf.SetMaxAge(6 * time.Hour)
	c.Check(rf.timestampFormat(), Equals, "20060102T15")
	rf.SetMaxAge(8 * 24 * time.Hour)
	c.Check(rf.timestampFormat(), Equals, "20060102")
	rf.SetMaxAge(90 * 24 * time.Hour)
	c.Check(rf.timestampFormat(), Equals, "200601")
}

func (a *LogRotateSuite) TestRotate(c *C) {
	fn := tempFile(".log")
	defer os.Remove(fn)
	rf, _ := Open(fn, 15 * time.Minute, 1024, 2)
	rf.Write([]byte("line1\n"))
	rfn, ts, err := rf.rotateOnly()
	c.Check(err, IsNil)
	c.Check(rfn, Matches, `^.*/[0-9a-f]{32}-[0-9]{8}T[0-9]{4}_x[0-9a-f]{16}\.log$`)
	c.Check(ts, Matches, `^[0-9]{8}T[0-9]{4}$`)
	_, err = os.Stat(fn)
	c.Check(err, NotNil)
	st, err := os.Stat(rfn)
	c.Check(err, IsNil)
	c.Check(st.Size(), Equals, int64(16 + len(fn)))
	defer os.Remove(rfn)
	rf.Write([]byte("line2\n"))
	rand.Seed(2)
	now := time.Now().In(time.Local)
	reg := regexp.MustCompile(`\.log$`)
	xrfn := reg.ReplaceAllString(fn, fmt.Sprintf("-%s_x2f8282cbe2f9696f.log", now.Format("20060102T1504")))
	f, err := os.Create(xrfn)
	c.Assert(err, IsNil)
	f.Close()
	defer os.Remove(xrfn)
	rfn, ts, err = rf.rotateOnly()
	c.Check(err, NotNil)
	rf.Close()
	rfn, ts, err = rf.rotateOnly()
	c.Check(err, NotNil)
}

func (a *LogRotateSuite) TestCompress(c *C) {
	fn := tempFile(".log")
	defer os.Remove(fn)
	rf, _ := Open(fn, 15 * time.Minute, 1024, 2)
	rf.Write([]byte("line1\n"))
	rf.Write([]byte("line2\n"))
	rf.Write([]byte("line3\n"))
	rf.Write([]byte("line4\n"))
	rf.Write([]byte("line5\n"))
	rf.Write([]byte("line6\n"))
	rf.Write([]byte("line7\n"))
	rf.Write([]byte("line8\n"))
	rf.Write([]byte("line9\n"))
	rf.Write([]byte("line10\n"))
	rfn, ts, err := rf.rotateOnly()
	c.Assert(err, IsNil)
	c.Assert(rfn, Not(Equals), "")
	gzfn, err := rf.compress(rfn, ts)
	c.Check(err, IsNil)
	c.Check(gzfn, Not(Equals), "")
	defer os.Remove(gzfn)
	st, err := os.Stat(gzfn)
	c.Assert(err, IsNil)
	c.Check(st.Size() < int64(71 + len(fn)), Equals, true)
}

func (a *LogRotateSuite) TestCleanup(c *C) {
	fn := tempFile(".log")
	defer os.Remove(fn)
	rf, _ := Open(fn, 15 * time.Minute, 1024, 0)
	gzfns := make([]string, 4)
	now := time.Now().Add(-4 * time.Hour)
	rf.Write([]byte("line1\n"))
	rf.start = &now
	rfn, ts, _ := rf.rotateOnly()
	c.Assert(rfn, Not(Equals), "")
	gzfns[0], _ = rf.compress(rfn, ts)
	rf.Write([]byte("line2\n"))
	now = now.Add(time.Hour)
	rf.start = &now
	rfn, ts, _ = rf.rotateOnly()
	c.Assert(rfn, Not(Equals), "")
	gzfns[1], _ = rf.compress(rfn, ts)
	rf.Write([]byte("line3\n"))
	now = now.Add(time.Hour)
	rf.start = &now
	rfn, ts, _ = rf.rotateOnly()
	c.Assert(rfn, Not(Equals), "")
	gzfns[2], _ = rf.compress(rfn, ts)
	rf.Write([]byte("line4\n"))
	now = now.Add(time.Hour)
	rf.start = &now
	rfn, ts, _ = rf.rotateOnly()
	c.Assert(rfn, Not(Equals), "")
	gzfns[3], _ = rf.compress(rfn, ts)
	rf.Write([]byte("line5\n"))
	for _, gzfn := range gzfns {
		c.Log(gzfn, " expect exists")
		_, err := os.Stat(gzfn)
		c.Check(err, IsNil)
		defer os.Remove(gzfn)
	}
	err := rf.cleanup()
	c.Check(err, IsNil)
	for _, gzfn := range gzfns {
		c.Log(gzfn, " expect exists")
		_, err := os.Stat(gzfn)
		c.Check(err, IsNil)
		defer os.Remove(gzfn)
	}
	rf.SetMaxBackups(5)
	err = rf.cleanup()
	c.Check(err, IsNil)
	for _, gzfn := range gzfns {
		c.Log(gzfn, " expect exists")
		_, err := os.Stat(gzfn)
		c.Check(err, IsNil)
		defer os.Remove(gzfn)
	}
	rf.SetMaxBackups(2)
	err = rf.cleanup()
	c.Check(err, IsNil)
	for _, gzfn := range gzfns[:2] {
		c.Log(gzfn, " expect gone")
		_, err := os.Stat(gzfn)
		c.Check(err, NotNil)
	}
	for _, gzfn := range gzfns[2:] {
		c.Log(gzfn, " expect exists")
		_, err := os.Stat(gzfn)
		c.Check(err, IsNil)
	}
}
