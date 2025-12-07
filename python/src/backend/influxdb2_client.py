"""
InfluxDB2 Client - Time series database client for storing power monitoring data

This module provides a client for writing time series data to InfluxDB2:
- Grid power consumption
- Electricity spot prices
- Solar power production

Features:
- Automatic reconnection on connection loss
- Environment variable configuration
- Batched writes for efficiency
- Error handling and logging
"""

import os
from typing import Optional, Dict, Any
from datetime import datetime
from dotenv import load_dotenv
import time


class InfluxDB2Client:
    """
    Client for writing time series data to InfluxDB2
    
    Handles automatic reconnection and batched writes for power monitoring data.
    """
    
    def __init__(
        self,
        host: Optional[str] = None,
        port: Optional[int] = None,
        user: Optional[str] = None,
        password: Optional[str] = None,
        org: str = "sotehus",
        bucket: str = "sotehus_bucket",
        token: Optional[str] = None
    ):
        """
        Initialize the InfluxDB2 client
        
        Args:
            host: InfluxDB host (if None, will load from environment)
            port: InfluxDB port (if None, will load from environment)
            user: InfluxDB username (if None, will load from environment)
            password: InfluxDB password (if None, will load from environment)
            org: InfluxDB organization name
            bucket: InfluxDB bucket name
            token: InfluxDB authentication token (optional, will use user/password if not provided)
        """
        # Load environment variables
        load_dotenv()
        
        self.host = host or os.getenv('INFLUXDB2_HOST')
        self.port = port or int(os.getenv('INFLUXDB2_PORT', '8086'))
        self.user = user or os.getenv('INFLUXDB2_USER')
        self.password = password or os.getenv('INFLUXDB2_PASSWORD')
        self.org = org
        self.bucket = bucket
        self.token = token
        
        # Validate required parameters
        if not self.host:
            raise ValueError("InfluxDB host is required. Set INFLUXDB2_HOST in .env file or pass as parameter.")
        
        # Initialize connection state
        self._client = None
        self._write_api = None
        self._connected = False
        self._last_connection_attempt = 0
        self._reconnect_delay = 5  # seconds
        
        # Try initial connection
        self._connect()
    
    def _connect(self) -> bool:
        """
        Establish connection to InfluxDB2
        
        Returns:
            True if connection successful, False otherwise
        """
        try:
            # Import here to avoid requiring influxdb-client if not used
            from influxdb_client import InfluxDBClient
            from influxdb_client.client.write_api import SYNCHRONOUS
            
            # Build connection URL
            url = f"http://{self.host}:{self.port}"
            
            # Use token if provided, otherwise use username/password
            if self.token:
                self._client = InfluxDBClient(
                    url=url,
                    token=self.token,
                    org=self.org
                )
            elif self.user and self.password:
                # For InfluxDB 2.x, use username and password directly
                # The client will handle authentication
                self._client = InfluxDBClient(
                    url=url,
                    username=self.user,
                    password=self.password,
                    org=self.org
                )
            else:
                print("InfluxDB: No authentication provided (token or user/password)")
                return False
            
            # Create write API
            self._write_api = self._client.write_api(write_options=SYNCHRONOUS)
            
            # Test connection by checking health
            health = self._client.health()
            if health.status == "pass":
                self._connected = True
                print(f"InfluxDB: Connected to {url} (org: {self.org}, bucket: {self.bucket})")
                return True
            else:
                print(f"InfluxDB: Health check failed - {health.message}")
                return False
                
        except ImportError:
            print("InfluxDB: influxdb-client package not installed. Run: pip install influxdb-client")
            return False
        except Exception as e:
            print(f"InfluxDB: Connection error - {e}")
            self._connected = False
            return False
    
    def _ensure_connection(self) -> bool:
        """
        Ensure connection is active, reconnect if necessary
        
        Returns:
            True if connected, False otherwise
        """
        if self._connected and self._client:
            return True
        
        # Implement exponential backoff for reconnection attempts
        current_time = time.time()
        if current_time - self._last_connection_attempt < self._reconnect_delay:
            return False
        
        self._last_connection_attempt = current_time
        print("InfluxDB: Attempting to reconnect...")
        
        if self._connect():
            print("InfluxDB: Reconnection successful")
            return True
        else:
            # Increase reconnect delay (exponential backoff, max 60 seconds)
            self._reconnect_delay = min(self._reconnect_delay * 2, 60)
            print(f"InfluxDB: Reconnection failed, will retry in {self._reconnect_delay}s")
            return False
    
    def write_power_data(
        self,
        grid_power: Optional[float] = None,
        spot_price: Optional[float] = None,
        solar_production: Optional[float] = None,
        timestamp: Optional[datetime] = None
    ) -> bool:
        """
        Write power monitoring data to InfluxDB
        
        Args:
            grid_power: Current grid power consumption in Watts
            spot_price: Current electricity spot price in SEK/kWh
            solar_production: Current solar power production in Watts
            timestamp: Timestamp for the data point (defaults to now)
            
        Returns:
            True if write successful, False otherwise
        """
        if not self._ensure_connection():
            return False
        
        # Use current time if not provided
        if timestamp is None:
            timestamp = datetime.utcnow()
        
        try:
            from influxdb_client import Point
            
            # Create a point with measurement name "power_monitoring"
            point = Point("power_monitoring").time(timestamp)
            
            # Add fields (only include non-None values)
            if grid_power is not None:
                point.field("grid_power", float(grid_power))
            
            if spot_price is not None:
                point.field("spot_price", float(spot_price))
            
            if solar_production is not None:
                point.field("solar_production", float(solar_production))
            
            # Write to InfluxDB
            self._write_api.write(bucket=self.bucket, org=self.org, record=point)
            
            return True
            
        except Exception as e:
            print(f"InfluxDB: Write error - {e}")
            self._connected = False  # Mark as disconnected to trigger reconnect
            return False
    
    def close(self):
        """Close the connection to InfluxDB"""
        if self._client:
            try:
                self._client.close()
                print("InfluxDB: Connection closed")
            except Exception as e:
                print(f"InfluxDB: Error closing connection - {e}")
            finally:
                self._connected = False
                self._client = None
                self._write_api = None
    
    def is_connected(self) -> bool:
        """Check if client is connected to InfluxDB"""
        return self._connected
    
    def __enter__(self):
        """Context manager entry"""
        return self
    
    def __exit__(self, exc_type, exc_val, exc_tb):
        """Context manager exit"""
        self.close()


# Example usage
if __name__ == "__main__":
    try:
        # Create client instance
        print("Initializing InfluxDB2 client...")
        client = InfluxDB2Client()
        
        if client.is_connected():
            print("Connected successfully!")
            
            # Write sample data
            print("\nWriting sample data...")
            success = client.write_power_data(
                grid_power=1500.5,
                spot_price=0.85,
                solar_production=2300.0
            )
            
            if success:
                print("Data written successfully!")
            else:
                print("Failed to write data")
        else:
            print("Failed to connect to InfluxDB")
        
        # Close connection
        client.close()
        
    except Exception as e:
        print(f"Error: {e}")
