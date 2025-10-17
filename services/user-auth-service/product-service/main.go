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

type Product struct {
	ID          string    `bson:"_id,omitempty" json:"id"`
	Name        string    `bson:"name" json:"name"`
	Description string    `bson:"description" json:"description"`
	Price       float64   `bson:"price" json:"price"`
	Category    string    `bson:"category" json:"category"`
	Stock       int       `bson:"stock" json:"stock"`
	Rating      float64   `bson:"rating" json:"rating"`
	Reviews     int       `bson:"reviews" json:"reviews"`
	ImageURL    string    `bson:"image_url" json:"image_url"`
	CreatedAt   time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time `bson:"updated_at" json:"updated_at"`
}

type ProductService struct {
	db *mongo.Database
}

var productService *ProductService

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
	productService = &ProductService{db: db}

	router := gin.Default()

	// Health Check
	router.GET("/health", healthCheck)
	router.GET("/ready", readinessCheck)

	// Product Routes
	router.GET("/api/v1/products", listProducts)
	router.GET("/api/v1/products/:id", getProduct)
	router.POST("/api/v1/products", createProduct)
	router.PUT("/api/v1/products/:id", updateProduct)
	router.DELETE("/api/v1/products/:id", deleteProduct)
	router.GET("/api/v1/products/search", searchProducts)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8002"
	}

	log.Printf("Product Service starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
		"service": "product-service",
		"timestamp": time.Now(),
	})
}

func readinessCheck(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := productService.db.Client().Ping(ctx, nil)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not ready",
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "ready",
		"service": "product-service",
	})
}

func listProducts(c *gin.Context) {
	collection := productService.db.Collection("products")
	
	opts := options.Find().SetLimit(20)
	cursor, err := collection.Find(context.Background(), bson.M{}, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch products"})
		return
	}
	defer cursor.Close(context.Background())

	var products []Product
	if err = cursor.All(context.Background(), &products); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode products"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"products": products,
		"count": len(products),
	})
}

func getProduct(c *gin.Context) {
	id := c.Param("id")
	collection := productService.db.Collection("products")

	var product Product
	err := collection.FindOne(context.Background(), bson.M{"_id": id}).Decode(&product)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
		return
	}

	c.JSON(http.StatusOK, product)
}

func createProduct(c *gin.Context) {
	var product Product
	if err := c.ShouldBindJSON(&product); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	product.CreatedAt = time.Now()
	product.UpdatedAt = time.Now()

	collection := productService.db.Collection("products")
	result, err := collection.InsertOne(context.Background(), product)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create product"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Product created successfully",
		"product_id": result.InsertedID,
	})
}

func updateProduct(c *gin.Context) {
	id := c.Param("id")
	var product Product
	if err := c.ShouldBindJSON(&product); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	product.UpdatedAt = time.Now()
	collection := productService.db.Collection("products")
	
	_, err := collection.UpdateOne(
		context.Background(),
		bson.M{"_id": id},
		bson.M{"$set": product},
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update product"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Product updated successfully"})
}

func deleteProduct(c *gin.Context) {
	id := c.Param("id")
	collection := productService.db.Collection("products")

	result, err := collection.DeleteOne(context.Background(), bson.M{"_id": id})
	if err != nil || result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Product deleted successfully"})
}

func searchProducts(c *gin.Context) {
	query := c.Query("q")
	collection := productService.db.Collection("products")

	opts := options.Find().SetLimit(20)
	cursor, err := collection.Find(context.Background(), bson.M{
		"$or": []bson.M{
			{"name": bson.M{"$regex": query, "$options": "i"}},
			{"description": bson.M{"$regex": query, "$options": "i"}},
		},
	}, opts)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Search failed"})
		return
	}
	defer cursor.Close(context.Background())

	var products []Product
	if err = cursor.All(context.Background(), &products); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode products"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"products": products,
		"count": len(products),
	})
}