package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// APIKey represents an API key with its metadata
type APIKey struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	KeyHash   string     `json:"-"` // Never expose the actual key hash in JSON
	Scope     []string   `json:"scope"`
	RateLimit int        `json:"rate_limit"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	IsActive  bool       `json:"is_active"`
}

// KeyMetadata represents metadata about an API key (excluding sensitive data)
type KeyMetadata struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Scope     []string   `json:"scope"`
	RateLimit int        `json:"rate_limit"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	IsActive  bool       `json:"is_active"`
}

// APIKeyInfo holds API key configuration
type APIKeyInfo struct {
	RateLimit      int
	Scope          []string
	AllowedChains  []string
	AllowedMethods []string
	ExpiresAt      *time.Time
	IsActive       bool
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	// Master key for signing API keys
	MasterKey string

	// Token expiration
	TokenExpiration time.Duration

	// Rate limiting
	DefaultRateLimit int

	// Maximum keys per user (if implementing multi-user)
	MaxKeysPerUser int
}

// AuthManager manages API key authentication
type AuthManager struct {
	config   AuthConfig
	keys     map[string]*APIKey
	keyIndex map[string]string // Maps plain key to hashed key
}

// NewAuthManager creates a new authentication manager
func NewAuthManager(config AuthConfig) *AuthManager {
	return &AuthManager{
		config:   config,
		keys:     make(map[string]*APIKey),
		keyIndex: make(map[string]string),
	}
}

// GetConfig returns the authentication configuration
func (am *AuthManager) GetConfig() AuthConfig {
	return am.config
}

// GenerateAPIKey creates a new API key
func (am *AuthManager) GenerateAPIKey(name string, scope []string, rateLimit int) (string, *KeyMetadata, error) {
	if !am.config.MasterKeySet() {
		return "", nil, errors.New("master key not configured")
	}

	if len(scope) == 0 {
		return "", nil, errors.New("scope cannot be empty")
	}

	// Generate unique API key
	apiKey := am.generateKey(name, rateLimit)

	// Hash the key for storage
	keyHash := am.hashKey(apiKey)

	// Create API key record
	now := time.Now()
	apiKeyRecord := &APIKey{
		ID:        uuid.New().String(),
		Name:      name,
		KeyHash:   keyHash,
		Scope:     scope,
		RateLimit: rateLimit,
		CreatedAt: now,
		UpdatedAt: now,
		IsActive:  true,
	}

	// Store the key
	am.keys[apiKeyRecord.ID] = apiKeyRecord
	am.keyIndex[apiKey] = apiKeyRecord.ID

	return apiKey, apiKeyRecord.ToMetadata(), nil
}

// ValidateAPIKey validates an API key
func (am *AuthManager) ValidateAPIKey(apiKey string) (*APIKeyInfo, error) {
	if apiKey == "" {
		return nil, errors.New("API key is required")
	}

	// Look up the key hash
	keyID, exists := am.keyIndex[apiKey]
	if !exists {
		return nil, errors.New("invalid API key")
	}

	// Get the key record
	keyRecord, exists := am.keys[keyID]
	if !exists {
		return nil, errors.New("API key not found")
	}

	// Verify the key
	if keyRecord.KeyHash != am.hashKey(apiKey) {
		return nil, errors.New("API key verification failed")
	}

	// Check if key is active
	if !keyRecord.IsActive {
		return nil, errors.New("API key is disabled")
	}

	// Check expiration
	if keyRecord.ExpiresAt != nil && time.Now().After(*keyRecord.ExpiresAt) {
		return nil, errors.New("API key has expired")
	}

	return &APIKeyInfo{
		RateLimit: keyRecord.RateLimit,
		Scope:     keyRecord.Scope,
		ExpiresAt: keyRecord.ExpiresAt,
		IsActive:  keyRecord.IsActive,
	}, nil
}

// RevokeAPIKey revokes an API key
func (am *AuthManager) RevokeAPIKey(apiKey string) error {
	if apiKey == "" {
		return errors.New("API key is required")
	}

	// Look up the key
	keyID, exists := am.keyIndex[apiKey]
	if !exists {
		return errors.New("API key not found")
	}

	// Mark as inactive
	if keyRecord, exists := am.keys[keyID]; exists {
		keyRecord.IsActive = false
		keyRecord.UpdatedAt = time.Now()
	}

	// Remove from index (key becomes unusable)
	delete(am.keyIndex, apiKey)

	return nil
}

// GetKeyMetadata returns metadata for an API key
func (am *AuthManager) GetKeyMetadata(apiKey string) (*KeyMetadata, error) {
	if apiKey == "" {
		return nil, errors.New("API key is required")
	}

	keyID, exists := am.keyIndex[apiKey]
	if !exists {
		return nil, errors.New("API key not found")
	}

	keyRecord, exists := am.keys[keyID]
	if !exists {
		return nil, errors.New("API key not found")
	}

	return keyRecord.ToMetadata(), nil
}

// ListAPIKeys lists all API keys
func (am *AuthManager) ListAPIKeys() []*KeyMetadata {
	keys := make([]*KeyMetadata, 0, len(am.keys))
	for _, key := range am.keys {
		keys = append(keys, key.ToMetadata())
	}
	return keys
}

// HasScope checks if the API key has the required scope
func (am *AuthManager) HasScope(apiKey string, requiredScope string) (bool, error) {
	info, err := am.ValidateAPIKey(apiKey)
	if err != nil {
		return false, err
	}

	for _, scope := range info.Scope {
		if scope == requiredScope || scope == "*" {
			return true, nil
		}
	}

	return false, nil
}

// CanAccessChain checks if the API key can access the specified chain
func (am *AuthManager) CanAccessChain(apiKey string, chain string) (bool, error) {
	info, err := am.ValidateAPIKey(apiKey)
	if err != nil {
		return false, err
	}

	// If no specific chains are defined, allow all
	if len(info.AllowedChains) == 0 {
		return true, nil
	}

	for _, allowedChain := range info.AllowedChains {
		if allowedChain == chain {
			return true, nil
		}
	}

	return false, fmt.Errorf("API key does not have access to chain: %s", chain)
}

// CanUseMethod checks if the API key can use the specified method
func (am *AuthManager) CanUseMethod(apiKey string, method string) (bool, error) {
	info, err := am.ValidateAPIKey(apiKey)
	if err != nil {
		return false, err
	}

	// If no specific methods are defined, allow all
	if len(info.AllowedMethods) == 0 {
		return true, nil
	}

	for _, allowedMethod := range info.AllowedMethods {
		if allowedMethod == method {
			return true, nil
		}
	}

	return false, fmt.Errorf("API key does not have permission to use method: %s", method)
}

// generateKey generates a new API key
func (am *AuthManager) generateKey(name string, rateLimit int) string {
	// Create a random seed using timestamp and random data
	timestamp := time.Now().UnixNano()
	uuid := uuid.New().String()

	data := fmt.Sprintf("%s:%d:%s:%s", name, rateLimit, timestamp, uuid)

	// Sign with master key
	signature := am.signData(data)

	// Combine into final key
	apiKey := fmt.Sprintf("bg_%s_%s", hex.EncodeToString([]byte(data[:16])), signature[:16])

	return apiKey
}

// hashKey hashes an API key for storage
func (am *AuthManager) hashKey(key string) string {
	mac := hmac.New(sha256.New, []byte(am.config.MasterKey))
	mac.Write([]byte(key))
	return hex.EncodeToString(mac.Sum(nil))
}

// signData signs data with the master key
func (am *AuthManager) signData(data string) string {
	mac := hmac.New(sha256.New, []byte(am.config.MasterKey))
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

// verifySignature verifies a signature
func (am *AuthManager) verifySignature(data, signature string) bool {
	expectedSignature := am.signData(data)
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

// ToMetadata converts APIKey to KeyMetadata
func (ak *APIKey) ToMetadata() *KeyMetadata {
	return &KeyMetadata{
		ID:        ak.ID,
		Name:      ak.Name,
		Scope:     ak.Scope,
		RateLimit: ak.RateLimit,
		ExpiresAt: ak.ExpiresAt,
		CreatedAt: ak.CreatedAt,
		IsActive:  ak.IsActive,
	}
}

// ConfigWithMasterKey creates a new AuthConfig with master key
func ConfigWithMasterKey(masterKey string) AuthConfig {
	return AuthConfig{
		MasterKey:        masterKey,
		TokenExpiration:  24 * time.Hour,
		DefaultRateLimit: 1000,
		MaxKeysPerUser:   10,
	}
}

// MasterKeySet checks if master key is configured
func (ac AuthConfig) MasterKeySet() bool {
	return ac.MasterKey != ""
}

// LoadAPIKeys loads API keys from storage (implement persistence as needed)
func (am *AuthManager) LoadAPIKeys(keys map[string]*APIKey) {
	am.keys = keys
	// Rebuild index
	am.keyIndex = make(map[string]string)
	for range keys {
		// Note: We can't reconstruct the plain key from hash
		// This would require persistent storage of the keys
		// The index will need to be rebuilt from the stored keys
	}
}

// ExportKeys exports API keys (be careful with this!)
func (am *AuthManager) ExportKeys() map[string]*APIKey {
	// Return a copy of the keys map
	keys := make(map[string]*APIKey)
	for k, v := range am.keys {
		keys[k] = v
	}
	return keys
}
