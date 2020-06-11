package helper

import (
	"fmt"
	"path"
	"testing"
)

func TestBase(t *testing.T) {
	base := path.Base("C:\\Users\\lairobin\\.btfs")
	fmt.Println("base", base)
}
