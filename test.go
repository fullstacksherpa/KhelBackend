package main

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

func main() {

	storedHash := "\x2432612431302436327a5969635470397872534f37473354737567374f596641436c6479573071336c4c474b736f443854536466497a30596b6c696d"
	plain := "khel"

	err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(plain))
	if err != nil {
		fmt.Println("FAIL:", err)
	} else {
		fmt.Println("SUCCESS")
	}
}
