import os
import json
import logging
from typing import Optional, Callable, Any, Dict
from datetime import datetime
import paho.mqtt.client as mqtt
from dotenv import load_dotenv


# Load environment variables from .env file
load_dotenv()


class MQTTPowerClient:
    """
    MQTT client for reading power consumption data
    """
    
    def __init__(self, 
                 broker_host: Optional[str] = None,
                 broker_port: int = 1883,
                 username: Optional[str] = None,
                 password: Optional[str] = None,
                 topic: Optional[str] = None) -> None:
        """
        Initialize the MQTT Power Client
        
        Args:
            broker_host: MQTT broker hostname (defaults to env MQTT_BROKER_HOST)
            broker_port: MQTT broker port (defaults to env MQTT_BROKER_PORT or 1883)
            username: MQTT username (defaults to env MQTT_USERNAME)
            password: MQTT password (defaults to env MQTT_PASSWORD)
            topic: MQTT topic to subscribe to (defaults to env MQTT_TOPIC)
        """
        # Load from environment variables if not provided
        self.broker_host = broker_host or os.getenv('MQTT_BROKER_HOST')
        self.broker_port = int(os.getenv('MQTT_BROKER_PORT', broker_port))
        self.username = username or os.getenv('MQTT_USERNAME')
        self.password = password or os.getenv('MQTT_PASSWORD')
        self.topic = topic or os.getenv('MQTT_TOPIC')
        
        # Validate required parameters
        if not self.broker_host:
            raise ValueError("MQTT broker host must be provided via parameter or MQTT_BROKER_HOST env variable")
        if not self.topic:
            raise ValueError("MQTT topic must be provided via parameter or MQTT_TOPIC env variable")
        
        # Ensure we have valid strings (mypy satisfaction)
        assert self.broker_host is not None
        assert self.topic is not None
        
        # MQTT client setup
        self.client = mqtt.Client()
        self.client.on_connect = self._on_connect
        self.client.on_message = self._on_message
        self.client.on_disconnect = self._on_disconnect
        
        # Set credentials if provided
        if self.username and self.password:
            self.client.username_pw_set(self.username, self.password)
        
        # Current power consumption
        self.current_power: Optional[float] = None
        self.last_updated: Optional[datetime] = None
        self.is_connected: bool = False
        
        # Callback for power updates
        self.power_callback: Optional[Callable[[float], None]] = None
        
        # Setup logging
        self.logger = logging.getLogger(__name__)
    
    def set_power_callback(self, callback: Callable[[float], None]) -> None:
        """
        Set a callback function to be called when power data is received
        
        Args:
            callback: Function that takes a float (power value) as parameter
        """
        self.power_callback = callback
    
    def connect(self) -> bool:
        """
        Connect to the MQTT broker
        
        Returns:
            True if connection successful, False otherwise
        """
        try:
            self.logger.info(f"Connecting to MQTT broker at {self.broker_host}:{self.broker_port}")
            # Type assertion since we validated these are not None in __init__
            assert self.broker_host is not None
            self.client.connect(self.broker_host, self.broker_port, 60)
            self.client.loop_start()  # Start the loop in a separate thread
            return True
        except Exception as e:
            self.logger.error(f"Failed to connect to MQTT broker: {e}")
            return False
    
    def disconnect(self) -> None:
        """Disconnect from the MQTT broker"""
        self.client.loop_stop()
        self.client.disconnect()
        self.is_connected = False
        self.logger.info("Disconnected from MQTT broker")
    
    def _on_connect(self, client: mqtt.Client, userdata: Any, flags: Dict[str, Any], rc: int) -> None:
        """Callback for when the client receives a CONNACK response from the server"""
        if rc == 0:
            self.is_connected = True
            self.logger.info(f"Connected to MQTT broker, subscribing to topic: {self.topic}")
            # Type assertion since we validated topic is not None in __init__
            assert self.topic is not None
            client.subscribe(self.topic)
        else:
            self.logger.error(f"Failed to connect to MQTT broker, return code {rc}")
    
    def _on_message(self, client: mqtt.Client, userdata: Any, msg: mqtt.MQTTMessage) -> None:
        """Callback for when a PUBLISH message is received from the server"""
        try:
            # Decode the message and convert directly to float
            message = msg.payload.decode('utf-8').strip()
            self.logger.debug(f"Received message on topic {msg.topic}: {message}")
            
            # Convert message directly to float
            power_value = float(message)
            
            self.current_power = power_value
            self.last_updated = datetime.now()
            self.logger.info(f"Updated power consumption: {power_value:.2f}W")
            
            # Call the callback if set
            if self.power_callback:
                self.power_callback(power_value)
                
        except ValueError as e:
            self.logger.warning(f"Could not parse power value from message: {message} - {e}")
        except Exception as e:
            self.logger.error(f"Error processing MQTT message: {e}")
    
    def _on_disconnect(self, client: mqtt.Client, userdata: Any, rc: int) -> None:
        """Callback for when the client disconnects from the broker"""
        self.is_connected = False
        if rc != 0:
            self.logger.warning(f"Unexpected MQTT disconnection, return code: {rc}")
        else:
            self.logger.info("MQTT client disconnected")
    
    def get_current_power(self) -> Optional[float]:
        """
        Get the current power consumption
        
        Returns:
            Current power in watts, or None if no data available
        """
        return self.current_power
    
    def get_last_updated(self) -> Optional[datetime]:
        """
        Get the timestamp of the last power update
        
        Returns:
            Datetime of last update, or None if no data received
        """
        return self.last_updated
    
    def get_connection_status(self) -> bool:
        """
        Get the connection status
        
        Returns:
            True if connected to MQTT broker, False otherwise
        """
        return self.is_connected