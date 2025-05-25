package db

import (
	"crypto/sha256"
	"encoding/base64"
)

type LoginModel struct {
	UserId         string `bson:"_id"`
	EmailId        string `bson:"email"`
	HashedPassword string `bson:"password"`
	CreatedOn      int64  `bson:"createdOn"`
}

func (m LoginModel) Id() string {
	if len(m.UserId) == 0 {
		m.UserId = generateShortUserID(m.EmailId)
	}

	return m.UserId
}

func (m LoginModel) CollectionName() string { return "login" }

func generateShortUserID(email string) string {
	hash := sha256.New()                                       // Create a new SHA-256 hash
	hash.Write([]byte(email))                                  // Write the email to the hash
	hashedBytes := hash.Sum(nil)                               // Get the resulting hash as a byte slice
	truncatedHash := hashedBytes[:8]                           // Truncate to the first 8 bytes
	userID := base64.URLEncoding.EncodeToString(truncatedHash) // Encode to a base64 string
	return userID
}
