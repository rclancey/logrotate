# Log rotation package for Go

[![GoDoc](http://godoc.org/github.com/rclancey/logrotate?status.svg)](http://godoc.org/github.com/rclancey/logrotate)

This package provides automatic log rotation for servers

## Installation

Use the `go` command:

	$ go get github.com/rclancey/logrotate

## Example

```go
package main

import (
    "log"
    "time"
)

func init() {
    logFn := "/var/log/myServer/errors.log"
    rotationPeriod := 24 * time.Hour
    maxSize := 0
    retainCount := 7
    rotlog, err := logrotate.Open(logFn, rotationPeriod, maxSize, retainCount)
    log.SetOutput(rotlog)
}
```

## Documentation

[Documentation](http://godoc.org/github.com/rclancey/logrotate) is hosted at GoDoc project.

## Copyright

Copyright (C) 2019-2020 by Ryan Clancey

Package released under MIT License.
See [LICENSE](https://github.com/rclancey/logrotate/blob/master/LICENSE) for details.
