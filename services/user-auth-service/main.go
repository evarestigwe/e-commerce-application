package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID       string `bson:"_id,omitempty" json:"id"`
	Email    string `bson:"email" json:"email"`
	Password string `bson:"password" json:"-"`
	Role     string `bson:"role" json:"role"`
	Name     string `bson:"name" json:"name"`
	Active   bool   `bson:"active" json:"active"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

type AuthService struct {
	db        *mongo.Database
	jwtSecret string
}

var authService *AuthService

func init() {
	gin.SetMode(os.Getenv("GIN_MODE"))
}

func main() {
	// MongoDB Connection
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer client.Disconnect(context.Background())

	db := client.Database("ecommerce")
	authService = &AuthService{
		db:        db,
		jwtSecret: os.Getenv("JWT_SECRET"),
	}

	if authService.jwtSecret == "" {
		authService.jwtSecret = "your-secret-key-change-in-production"
	}

	// Create indexes
	createIndexes(db)

	// Gin Router
	router := gin.Default()

	// Health Check
	router.GET("/health", healthCheck)
	router.GET("/ready", readinessCheck)

	// Auth Routes
	router.POST("/api/v1/auth/register", register)
	router.POST("/api/v1/auth/login", login)
	router.POST("/api/v1/auth/refresh", refreshToken)
	router.POST("/api/v1/auth/logout", logout)
	router.GET("/api/v1/auth/profile", authMiddleware, getProfile)
	router.PUT("/api/v1/auth/profile", authMiddleware, updateProfile)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8001"
	}

	log.Printf("User Auth Service starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func createIndexes(db *mongo.Database) {
	collection := db.Collection("users")
	indexModel := mongo.IndexModel{
		Keys: bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	}
	_, err := collection.Indexes().CreateOne(context.Background(), indexModel)
	if err != nil {
		log.Printf("Failed to create index: %v", err)
	}
}

func healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
		"service": "user-auth-service",
		"timestamp": time.Now(),
	})
}

func readinessCheck(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := authService.db.Client().Ping(ctx, nil)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not ready",
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "ready",
		"service": "user-auth-service",
	})
}

func register(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=8"`
		Name     string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	user := User{
		Email:     req.Email,
		Password:  string(hashedPassword),
		Name:      req.Name,
		Role:      "customer",
		Active:    true,
		CreatedAt: time.Now(),
	}

	collection := authService.db.Collection("users")
	result, err := collection.InsertOne(context.Background(), user)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Email already exists"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "User registered successfully",
		"user_id": result.InsertedID,
	})
}

func login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	collection := authService.db.Collection("users")
	var user User
	err := collection.FindOne(context.Background(), bson.M{"email": req.Email}).Decode(&user)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	// Verify password
	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	// Generate tokens
	accessToken, refreshToken, expiresIn := generateTokens(user.ID, user.Email, user.Role)

	c.JSON(http.StatusOK, TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    expiresIn,
	})
}

func refreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate refresh token
	token, err := jwt.Parse(req.RefreshToken, func(token *jwt.Token) (interface{}, error) {
		return []byte(authService.jwtSecret), nil
	})

	if err != nil || !token.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
		return
	}

	claims := token.Claims.(jwt.MapClaims)
	userID := claims["sub"].(string)
	email := claims["email"].(string)
	role := claims["role"].(string)

	accessToken, newRefreshToken, expiresIn := generateTokens(userID, email, role)

	c.JSON(http.StatusOK, TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    expiresIn,
	})
}

func logout(c *gin.Context) {
	// In production, add token to blacklist
	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}

func getProfile(c *gin.Context) {
	userID := c.GetString("user_id")
	
	collection := authService.db.Collection("users")
	var user User
	err := collection.FindOne(context.Background(), bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	c.JSON(http.StatusOK, user)
}

func updateProfile(c *gin.Context) {
	userID := c.GetString("user_id")
	
	var req struct {
		Name string `json:"name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	collection := authService.db.Collection("users")
	_, err := collection.UpdateOne(
		context.Background(),
		bson.M{"_id": userID},
		bson.M{"$set": bson.M{"name": req.Name}},
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update profile"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Profile updated successfully"})
}

func generateTokens(userID, email, role string) (string, string, int64) {
	accessTokenExpiry := time.Now().Add(15 * time.Minute)
	refreshTokenExpiry := time.Now().Add(7 * 24 * time.Hour)

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   userID,
		"email": email,
		"role":  role,
		"exp":   accessTokenExpiry.Unix(),
		"iat":   time.Now().Unix(),
	})

	accessTokenString, _ := accessToken.SignedString([]byte(authService.jwtSecret))

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   userID,
		"email": email,
		"role":  role,
		"exp":   refreshTokenExpiry.Unix(),
		"iat":   time.Now().Unix(),
	})

	refreshTokenString, _ := refreshToken.SignedString([]byte(authService.jwtSecret))

	return accessTokenString, refreshTokenString, accessTokenExpiry.Unix()
}

func authMiddleware(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing authorization header"})
		c.Abort()
		return
	}

	tokenString := authHeader[7:] // Remove "Bearer "
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(authService.jwtSecret), nil
	})

	if err != nil || !token.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
		c.Abort()
		return
	}

	claims := token.Claims.(jwt.MapClaims)
	c.Set("user_id", claims["sub"])
	c.Set("email", claims["email"])
	c.Set("role", claims["role"])
	c.Next()
}