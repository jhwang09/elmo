package random

import (
	"crypto/rand"
	"encoding/base64"
	"io"

	"github.com/jhwang09/elmo/errs"
)

func Uid(numChars int) (uid string, err errs.Err) {
	if numChars%4 != 0 {
		err = errs.NewError("uid length must be a multiple of 4")
		return
	}
	buf := make([]byte, numChars)
	_, stdErr := io.ReadFull(rand.Reader, buf)
	if stdErr != nil {
		err = errs.NewStdError(stdErr)
		return
	}

	uid = base64.URLEncoding.EncodeToString(buf)
	return
}
