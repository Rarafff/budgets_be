package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	defaultTokenDuration    = 24 * time.Hour
	rememberMeTokenDuration = 30 * 24 * time.Hour
	passwordResetDuration   = 30 * time.Minute
)

type User struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Email       string    `json:"email"`
	PhoneNumber string    `json:"phoneNumber"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type UserWithPassword struct {
	User
	PasswordHash string
}

type Repository interface {
	CreateUser(ctx context.Context, name, email, phoneNumber, passwordHash string) (User, error)
	FindUserByEmail(ctx context.Context, email string) (UserWithPassword, error)
	FindUserByID(ctx context.Context, id string) (User, error)
	UpdateProfile(ctx context.Context, userID, name, phoneNumber string) (User, error)
	UpdatePassword(ctx context.Context, userID, passwordHash string) error
	CreatePasswordResetToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	FindValidPasswordResetToken(ctx context.Context, tokenHash string) (string, error)
	MarkPasswordResetTokenUsed(ctx context.Context, tokenHash string) error
}

type Service struct {
	Repo      Repository
	JWTSecret string
}

type RegisterRequest struct {
	Name        string `json:"name"`
	Email       string `json:"email"`
	PhoneNumber string `json:"phoneNumber"`
	Password    string `json:"password"`
	RememberMe  bool   `json:"rememberMe"`
}

type LoginRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	RememberMe bool   `json:"rememberMe"`
}

type AuthResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
	User      User      `json:"user"`
}

type UpdateProfileRequest struct {
	Name        string `json:"name"`
	PhoneNumber string `json:"phoneNumber"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

type ForgotPasswordResponse struct {
	Message        string    `json:"message"`
	ResetToken     string    `json:"resetToken,omitempty"`
	ResetExpiresAt time.Time `json:"resetExpiresAt,omitempty"`
}

type ResetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"newPassword"`
}

func (s Service) Register(ctx context.Context, req RegisterRequest) (AuthResponse, error) {
	name := strings.TrimSpace(req.Name)
	email := normalizeEmail(req.Email)
	phoneNumber := strings.TrimSpace(req.PhoneNumber)
	if name == "" {
		return AuthResponse{}, errors.New("name is required")
	}
	if email == "" {
		return AuthResponse{}, errors.New("email is required")
	}
	if len(req.Password) < 8 {
		return AuthResponse{}, errors.New("password must be at least 8 characters")
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return AuthResponse{}, err
	}

	user, err := s.Repo.CreateUser(ctx, name, email, phoneNumber, string(passwordHash))
	if err != nil {
		return AuthResponse{}, err
	}

	token, expiresAt, err := s.CreateToken(user.ID, tokenDuration(req.RememberMe))
	if err != nil {
		return AuthResponse{}, err
	}

	return AuthResponse{Token: token, ExpiresAt: expiresAt, User: user}, nil
}

func (s Service) Login(ctx context.Context, req LoginRequest) (AuthResponse, error) {
	email := normalizeEmail(req.Email)
	if email == "" || req.Password == "" {
		return AuthResponse{}, errors.New("invalid email or password")
	}

	user, err := s.Repo.FindUserByEmail(ctx, email)
	if err != nil {
		return AuthResponse{}, errors.New("invalid email or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return AuthResponse{}, errors.New("invalid email or password")
	}

	token, expiresAt, err := s.CreateToken(user.ID, tokenDuration(req.RememberMe))
	if err != nil {
		return AuthResponse{}, err
	}

	return AuthResponse{Token: token, ExpiresAt: expiresAt, User: user.User}, nil
}

func (s Service) Profile(ctx context.Context, userID string) (User, error) {
	return s.Repo.FindUserByID(ctx, userID)
}

func (s Service) UpdateProfile(ctx context.Context, userID string, req UpdateProfileRequest) (User, error) {
	name := strings.TrimSpace(req.Name)
	phoneNumber := strings.TrimSpace(req.PhoneNumber)
	if name == "" {
		return User{}, errors.New("name is required")
	}

	return s.Repo.UpdateProfile(ctx, userID, name, phoneNumber)
}

func (s Service) ChangePassword(ctx context.Context, userID string, req ChangePasswordRequest) error {
	if req.CurrentPassword == "" {
		return errors.New("current password is required")
	}
	if len(req.NewPassword) < 8 {
		return errors.New("new password must be at least 8 characters")
	}

	user, err := s.Repo.FindUserByID(ctx, userID)
	if err != nil {
		return err
	}

	userWithPassword, err := s.Repo.FindUserByEmail(ctx, user.Email)
	if err != nil {
		return err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(userWithPassword.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		return errors.New("current password is incorrect")
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	return s.Repo.UpdatePassword(ctx, userID, string(passwordHash))
}

func (s Service) ForgotPassword(ctx context.Context, req ForgotPasswordRequest) (ForgotPasswordResponse, error) {
	email := normalizeEmail(req.Email)
	if email == "" {
		return ForgotPasswordResponse{}, errors.New("email is required")
	}

	generic := ForgotPasswordResponse{
		Message: "If that email exists, a password reset link has been created.",
	}

	user, err := s.Repo.FindUserByEmail(ctx, email)
	if err != nil {
		return generic, nil
	}

	token, tokenHash, err := generateResetToken()
	if err != nil {
		return ForgotPasswordResponse{}, err
	}

	expiresAt := time.Now().Add(passwordResetDuration)
	if err := s.Repo.CreatePasswordResetToken(ctx, user.ID, tokenHash, expiresAt); err != nil {
		return ForgotPasswordResponse{}, err
	}

	generic.ResetToken = token
	generic.ResetExpiresAt = expiresAt
	return generic, nil
}

func (s Service) ResetPassword(ctx context.Context, req ResetPasswordRequest) error {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return errors.New("reset token is required")
	}
	if len(req.NewPassword) < 8 {
		return errors.New("new password must be at least 8 characters")
	}

	tokenHash := hashResetToken(token)
	userID, err := s.Repo.FindValidPasswordResetToken(ctx, tokenHash)
	if err != nil {
		return errors.New("reset token is invalid or expired")
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	if err := s.Repo.UpdatePassword(ctx, userID, string(passwordHash)); err != nil {
		return err
	}

	return s.Repo.MarkPasswordResetTokenUsed(ctx, tokenHash)
}

func (s Service) CreateToken(userID string, duration time.Duration) (string, time.Time, error) {
	expiresAt := time.Now().Add(duration)
	claims := jwt.MapClaims{
		"sub": userID,
		"exp": expiresAt.Unix(),
		"iat": time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.JWTSecret))
	if err != nil {
		return "", time.Time{}, err
	}

	return tokenString, expiresAt, nil
}

func (s Service) ParseToken(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		return "", errors.New("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid token claims")
	}

	sub, err := claims.GetSubject()
	if err != nil || strings.TrimSpace(sub) == "" {
		return "", errors.New("invalid token subject")
	}

	return sub, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func tokenDuration(rememberMe bool) time.Duration {
	if rememberMe {
		return rememberMeTokenDuration
	}
	return defaultTokenDuration
}

func generateResetToken() (string, string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", err
	}

	token := hex.EncodeToString(bytes)
	return token, hashResetToken(token), nil
}

func hashResetToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
