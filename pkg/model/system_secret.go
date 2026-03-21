package model

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"

	"github.com/zxh326/kite/pkg/common"
	"gorm.io/gorm"
	"k8s.io/klog/v2"
)

// SystemSecret stores auto-generated application secrets in the database.
// Values are plain text (not SecretString) to avoid circular encryption.
type SystemSecret struct {
	Name  string `json:"name" gorm:"primaryKey;column:name;type:varchar(64)"`
	Value string `json:"value" gorm:"column:value;type:text;not null"`
}

const (
	secretNameJWT     = "jwt_secret"
	secretNameEncrypt = "encrypt_key"

	defaultJWTSecret  = "kite-default-jwt-secret-key-change-in-production"
	defaultEncryptKey = "kite-default-encryption-key-change-in-production"
)

// EnsureSecrets guarantees that JwtSecret and KiteEncryptKey hold
// cryptographically secure values. Must be called after InitDB()
// and before any code that reads SecretString columns.
//
// Priority: env var > DB stored value > auto-generated.
func EnsureSecrets() {
	common.JwtSecret = ensureOneSecret(
		secretNameJWT, common.JwtSecret, "JWT_SECRET", defaultJWTSecret, false,
		os.Getenv("JWT_SECRET") != "",
	)
	common.KiteEncryptKey = ensureOneSecret(
		secretNameEncrypt, common.KiteEncryptKey, "KITE_ENCRYPT_KEY", defaultEncryptKey, true,
		os.Getenv("KITE_ENCRYPT_KEY") != "",
	)
}

func ensureOneSecret(dbName, currentValue, envName, knownDefault string, isEncryptionKey, envWasSet bool) string {
	if envWasSet {
		return currentValue
	}

	stored, dbErr := loadSecret(dbName)
	if dbErr != nil {
		klog.Fatalf("Cannot read %s from database: %v (refusing to proceed with ambiguous secret state)", envName, dbErr)
	}
	if stored != "" {
		return stored
	}

	if isEncryptionKey && hasExistingEncryptedData() {
		effective := persistSecret(dbName, currentValue)
		klog.Warningf("════════════════════════════════════════════════════════════")
		klog.Warningf("  %s is using the insecure hardcoded default.", envName)
		klog.Warningf("  Existing encrypted data has been preserved.")
		klog.Warningf("  Please set %s to a secure random value", envName)
		klog.Warningf("  and re-encrypt your data.")
		klog.Warningf("════════════════════════════════════════════════════════════")
		return effective
	}

	secret := persistSecret(dbName, generateRandomSecret(32))
	klog.Infof("Auto-generated %s and stored in database (first boot)", envName)
	return secret
}

func generateRandomSecret(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		klog.Fatalf("Failed to generate random secret: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func loadSecret(name string) (string, error) {
	var s SystemSecret
	err := DB.Where("name = ?", name).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return s.Value, nil
}

// persistSecret inserts the secret if no row exists yet. If a row already
// exists the stored value is returned (first writer wins). Fatals on
// unrecoverable DB errors.
func persistSecret(name, value string) string {
	var existing SystemSecret
	err := DB.Where("name = ?", name).First(&existing).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := DB.Create(&SystemSecret{Name: name, Value: value}).Error; err != nil {
			if stored, readErr := loadSecret(name); readErr == nil && stored != "" {
				klog.Infof("Secret %q was created by another instance, adopting its value", name)
				return stored
			}
			klog.Fatalf("Failed to persist secret %q and no stored winner found: %v", name, err)
		}
		return value
	}
	if err != nil {
		klog.Fatalf("Failed to read secret %q from database: %v", name, err)
	}
	return existing.Value
}

// hasExistingEncryptedData returns true when the database contains rows with
// non-empty SecretString columns. Returns true on query errors (fail-safe).
func hasExistingEncryptedData() bool {
	checks := []struct {
		model interface{}
		where string
	}{
		{&Cluster{}, "config IS NOT NULL AND config != ''"},
		{&OAuthProvider{}, "client_secret IS NOT NULL AND client_secret != ''"},
		{&User{}, "api_key IS NOT NULL AND api_key != ''"},
		{&GeneralSetting{}, "ai_api_key IS NOT NULL AND ai_api_key != ''"},
	}
	for _, c := range checks {
		var count int64
		if err := DB.Model(c.model).Where(c.where).Count(&count).Error; err != nil {
			klog.Warningf("Failed to check for encrypted data (%T): %v — assuming data exists (fail-safe)", c.model, err)
			return true
		}
		if count > 0 {
			return true
		}
	}
	return false
}
