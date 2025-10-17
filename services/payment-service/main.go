package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Payment struct {
	ID        string    `bson:"_id,omitempty" json:"id"`
	OrderID   string    `bson:"order_id" json:"order_id"`
	UserID    string    `bson:"user_id" json:"user_id"`
	Amount    float64   `bson:"amount" json:"amount"`
	Currency  string    `bson:"currency" json:"currency"`
	Status    string    `bson:"status" json:"status"`
	Method    string    `bson:"method" json:"method"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}

type PaymentService struct {
	db *mongo.Database
}

var paymentService *PaymentService

func main() {
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
	paymentService = &PaymentService{db: db}

	router := gin.Default()

	router.GET("/health", healthCheck)
	router.GET("/ready", readinessCheck)

	router.POST("/api/v1/payments", processPayment)
	router.GET("/api/v1/payments/:id", getPayment)
	router.POST("/api/v1/payments/:id/refund", refundPayment)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8005"
	}

	log.Printf("Payment Service starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
		"service": "payment-service",
		"timestamp": time.Now(),
	})
}

func readinessCheck(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := paymentService.db.Client().Ping(ctx, nil)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not ready",
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "ready",
		"service": "payment-service",
	})
}

func processPayment(c *gin.Context) {
	var payment Payment
	if err := c.ShouldBindJSON(&payment); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	payment.Status = "processing"
	payment.CreatedAt = time.Now()
	payment.UpdatedAt = time.Now()

	// Simulate payment processing
	time.Sleep(1 * time.Second)
	payment.Status = "completed"

	collection := paymentService.db.Collection("payments")
	result, err := collection.InsertOne(context.Background(), payment)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process payment"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Payment processed successfully",
		"payment_id": result.InsertedID,
		"status": "completed",
	})
}

func getPayment(c *gin.Context) {
	id := c.Param("id")
	collection := paymentService.db.Collection("payments")

	var payment Payment
	err := collection.FindOne(context.Background(), bson.M{"_id": id}).Decode(&payment)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Payment not found"})
		return
	}

	c.JSON(http.StatusOK, payment)
}

func refundPayment(c *gin.Context) {
	id := c.Param("id")
	collection := paymentService.db.Collection("payments")

	_, err := collection.UpdateOne(
		context.Background(),
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"status": "refunded", "updated_at": time.Now()}},
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to refund payment"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Payment refunded successfully"})
}