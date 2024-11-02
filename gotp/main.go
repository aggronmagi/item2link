package main

import (
	"fmt"
	"os"

	"github.com/xlzd/gotp"
)

func main() {
	fmt.Println(gotp.NewDefaultTOTP(os.Args[1]).Now())
}
