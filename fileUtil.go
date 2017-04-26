package elmo

import (
	"os"

	"github.com/jhwang09/elmo/errs"
)

func Open(path string) (file *os.File, err errs.Err) {
	file, stdErr := os.Open(path)
	if stdErr != nil {
		err = errs.NewStdErrorWithInfo(stdErr, errs.Info{"Path": path})
		return
	}
	return
}
