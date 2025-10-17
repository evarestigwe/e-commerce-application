const express = require('express');
const { MongoClient } = require('mongodb');
const cors = require('cors');
const helmet = require('helmet');
const pino = require('pino');
const pinoHttp = require('pino-http');
require('dotenv').config();

const app = express();
const logger = pino();

// Middleware
app.use(helmet());
app.use(cors());
app.use(express.json());
app.use(pinoHttp({ logger }));

// MongoDB Connection
const mongoURI = process.env.MONGODB_URI || 'mongodb://localhost:27017';
let db;

const connectDB = async () => {
  try {
    const client = new MongoClient(mongoURI);
    await client.connect();
    db = client.db('ecommerce');
    logger.info('Connected to MongoDB');
  } catch (error) {
    logger.error('MongoDB connection failed:', error);
    process.exit(1);
  }
};

// Health Check
app.get('/health', (req, res) => {
  res.json({
    status: 'healthy',
    service: 'cart-service',
    timestamp: new Date(),
  });
});

// Readiness Check
app.get('/ready', async (req, res) => {
  try {
    await db.admin().ping();
    res.json({
      status: 'ready',
      service: 'cart-service',
    });
  } catch (error) {
    res.status(503).json({
      status: 'not ready',
      error: error.message,
    });
  }
});

// Get Cart
app.get('/api/v1/carts/:userId', async (req, res) => {
  try {
    const { userId } = req.params;
    const collection = db.collection('carts');
    
    let cart = await collection.findOne({ userId });
    if (!cart) {
      cart = { userId, items: [], total: 0, createdAt: new Date() };
      await collection.insertOne(cart);
    }
    
    res.json(cart);
  } catch (error) {
    logger.error('Error fetching cart:', error);
    res.status(500).json({ error: 'Failed to fetch cart' });
  }
});

// Add to Cart
app.post('/api/v1/carts/:userId/items', async (req, res) => {
  try {
    const { userId } = req.params;
    const { productId, quantity, price } = req.body;
    
    const collection = db.collection('carts');
    
    const result = await collection.updateOne(
      { userId },
      {
        $push: {
          items: {
            productId,
            quantity,
            price,
            addedAt: new Date(),
          },
        },
        $inc: { total: quantity * price },
      },
      { upsert: true }
    );
    
    res.json({ message: 'Item added to cart', result });
  } catch (error) {
    logger.error('Error adding to cart:', error);
    res.status(500).json({ error: 'Failed to add item to cart' });
  }
});

// Remove from Cart
app.delete('/api/v1/carts/:userId/items/:productId', async (req, res) => {
  try {
    const { userId, productId } = req.params;
    const collection = db.collection('carts');
    
    const cart = await collection.findOne({ userId });
    const item = cart.items.find(i => i.productId === productId);
    
    if (item) {
      await collection.updateOne(
        { userId },
        {
          $pull: { items: { productId } },
          $inc: { total: -(item.quantity * item.price) },
        }
      );
    }
    
    res.json({ message: 'Item removed from cart' });
  } catch (error) {
    logger.error('Error removing from cart:', error);
    res.status(500).json({ error: 'Failed to remove item from cart' });
  }
});

// Clear Cart
app.delete('/api/v1/carts/:userId', async (req, res) => {
  try {
    const { userId } = req.params;
    const collection = db.collection('carts');
    
    await collection.deleteOne({ userId });
    res.json({ message: 'Cart cleared' });
  } catch (error) {
    logger.error('Error clearing cart:', error);
    res.status(500).json({ error: 'Failed to clear cart' });
  }
});

// Start Server
const PORT = process.env.PORT || 8003;

connectDB().then(() => {
  app.listen(PORT, () => {
    logger.info(`Cart Service listening on port ${PORT}`);
  });
});