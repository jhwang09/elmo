package random

import (
	"math"
	"math/rand"
	"strconv"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// Between returns a random int in the range [min, lessThan)
func Between(min, lessThan int) int {
	return rand.Intn(lessThan-min) + min
}

// Digits returns a random number with numDigits digits
func Digits(numDigits int) string {
	min := int(math.Pow10(numDigits - 1))
	lessThan := int(math.Pow10(numDigits))
	return strconv.Itoa(Between(min, lessThan))
}
