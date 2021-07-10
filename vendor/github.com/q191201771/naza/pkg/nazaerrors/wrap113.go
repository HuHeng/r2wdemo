// Copyright 2021, Chef.  All rights reserved.
// https://github.com/q191201771/naza
//
// Use of this source code is governed by a MIT-style license
// that can be found in the License file.
//
// Author: Chef (191201771@qq.com)

// +build go1.13

package nazaerrors

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
)

func Wrap(err error) error {
	if err == nil {
		return nil
	}

	_, file, line, _ := runtime.Caller(1)
	s := filepath.Base(file)
	return fmt.Errorf("%w(%s:%d)", err, s, line)
}

func Unwrap(err error) error {
	return errors.Unwrap(err)
}

func Is(err, target error) bool {
	return errors.Is(err, target)
}

func As(err error, target interface{}) bool {
	return errors.As(err, target)
}
