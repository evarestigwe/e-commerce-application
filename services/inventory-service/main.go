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

type Inventory struct {
	ID        string    `bson:"_id,omitempty" json:"id"`
	ProductID string    `bson:"product_id" json:"product_id"`
	Quantity  int       `bson:"quantity" json:"quantity"`
	Reserved  int       `bson:"reserved" json:"reserved"`
	Warehouse string    `bson:"warehouse" json:"warehouse"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}

type InventoryService struct {
	db *mongo.Database
}

var inventoryService *InventoryService

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
	inventoryService = &InventoryService{db: db}

	router := gin.Default()

	router.GET("/health", healthCheck)
	router.GET("/ready", readinessCheck)

	router.GET("/api/v1/inventory/:productId", getInventory)
	router.POST("/api/v1/inventory", createInventory)
	router.PUT("/api/v1/inventory/:productId/reserve", reserveInventory)
	router.PUT("/api/v1/inventory/:productId/release", releaseInventory)
	router.PUT("/api/v1/inventory/:productId/update", updateInventory)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8006"
	}

	log.Printf("Inventory Service starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
		"service": "inventory-service",
		"timestamp": time.Now(),
	})
}

func readinessCheck(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := inventoryService.db.Client().Ping(ctx, nil)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not ready",
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "ready",
		"service": "inventory-service",
	})
}

func getInventory(c *gin.Context) {
	productID := c.Param("productId")
	collection := inventoryService.db.Collection("inventory")

	var inventory Inventory
	err := collection.FindOne(context.Background(), bson.M{"product_id": productID}).Decode(&inventory)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Inventory not found"})
		return
	}

	c.JSON(http.StatusOK, inventory)
}

func createInventory(c *gin.Context) {
	var inventory Inventory
	if err := c.ShouldBindJSON(&inventory); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	inventory.UpdatedAt = time.Now()
	collection := inventoryService.db.Collection("inventory")
	result, err := collection.InsertOne(context.Background(), inventory)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create inventory"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Inventory created successfully",
		"inventory_id": result.InsertedID,
	})
}

func reserveInventory(c *gin.Context) {
	productID := c.Param("productId")
	var req struct {
		Quantity int `json:"quantity" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	collection := inventoryService.db.Collection("inventory")
	result, err := collection.UpdateOne(
		context.Background(),
		bson.M{"product_id": productID, "quantity": bson.M{"$gte": req.Quantity}},
		bson.M{
			"$inc": bson.M{
				"quantity": -req.Quantity,
				"reserved": req.Quantity,
			},
			"$set": bson.M{"updated_at": time.Now()},
		},
	)

	if err != nil || result.ModifiedCount == 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Insufficient inventory"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Inventory reserved successfully"})
}

func releaseInventory(c *gin.Context) {
	productID := c.Param("productId")
	var req struct {
		Quantity int `json:"quantity" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	collection := inventoryService.db.Collection("inventory")
	_, err := collection.UpdateOne(
		context.Background(),
		bson.M{"product_id": productID},
		bson.M{
			"$inc": bson.M{
				"quantity": req.Quantity,
				"reserved": -req.Quantity,
			},
			"$set": bson.M{"updated_at": time.Now()},
		},
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to release inventory"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Inventory released successfully"})
}

func updateInventory(c *gin.Context) {
	productID := c.Param("productId")
	var req struct {
		Quantity int `json:"quantity" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	collection := inventoryService.db.Collection("inventory")
	_, err := collection.UpdateOne(
		context.Background(),
		bson.M{"product_id": productID},
		bson.M{
			"$set": bson.M{
				"quantity": req.Quantity,
				"updated_at": time.Now(),
			},
		},
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update inventory"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Inventory updated successfully"})
}