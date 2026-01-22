package orders

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type OrderNumberGenerator struct {
	secret string
}

func NewOrderNumberGenerator(secret string) *OrderNumberGenerator {
	return &OrderNumberGenerator{secret: secret}
}

func (g *OrderNumberGenerator) Generate(userID int64) string {
	nonce := uuid.NewString()

	mac := hmac.New(sha256.New, []byte(g.secret))
	mac.Write([]byte(fmt.Sprintf("uid:%d|nonce:%s", userID, nonce)))

	sum := mac.Sum(nil)
	tag := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum)

	return fmt.Sprintf(
		"KHEL-%s-%s",
		strings.ToUpper(tag[:4]),
		strings.ToUpper(uuid.NewString()[:4]),
	)
}
