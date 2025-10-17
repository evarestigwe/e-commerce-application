const express = require('express');
const { MongoClient } = require('mongodb');
const nodemailer = require('nodemailer');
const pino = require('pino');
const pinoHttp = require('pino-http');
require('dotenv').config();

const app = express();
const logger = pino();

app.use(express.json());
app.use(pinoHttp({ logger }));

let db;

const connectDB = async () => {
  try {
    const mongoURI = process.env.MONGODB_URI || 'mongodb://localhost:27017';
    const client = new MongoClient(mongoURI);
    await client.connect();
    db = client.db('ecommerce');
    logger.info('Connected to MongoDB');
  } catch (error) {
    logger.error('MongoDB connection failed:', error);
    process.exit(1);
  }
};

// Email transporter
const transporter = nodemailer.createTransport({
  service: process.env.EMAIL_SERVICE || 'gmail',
  auth: {
    user: process.env.EMAIL_USER,
    pass: process.env.EMAIL_PASSWORD,
  },
});

// Health Check
app.get('/health', (req, res) => {
  res.json({
    status: 'healthy',
    service: 'notification-service',
    timestamp: new Date(),
  });
});

// Readiness Check
app.get('/ready', async (req, res) => {
  try {
    await db.admin().ping();
    res.json({
      status: 'ready',
      service: 'notification-service',
    });
  } catch (error) {
    res.status(503).json({
      status: 'not ready',
      error: error.message,
    });
  }
});

// Send Email Notification
app.post('/api/v