"""
Tests for MQTTPowerClient - MQTT client for power consumption monitoring

Tests cover:
- Client initialization and configuration
- Connection handling
- Message parsing and callbacks
- Error handling
- State management
"""

import pytest
from unittest.mock import Mock, patch, MagicMock, call
from datetime import datetime
import paho.mqtt.client as mqtt
import os
from src.backend.mqtt_client import MQTTPowerClient


class TestMQTTPowerClientInit:
    """Test initialization and configuration"""
    
    def test_init_with_parameters(self):
        """Test initialization with explicit parameters"""
        client = MQTTPowerClient(
            broker_host="test.mqtt.com",
            broker_port=1883,
            username="user",
            password="pass",
            topic="test/topic"
        )
        
        assert client.broker_host == "test.mqtt.com"
        assert client.broker_port == 1883
        assert client.username == "user"
        assert client.password == "pass"
        assert client.topic == "test/topic"
    
    @patch.dict(os.environ, {
        'MQTT_BROKER_HOST': 'env.mqtt.com',
        'MQTT_BROKER_PORT': '8883',
        'MQTT_USERNAME': 'env_user',
        'MQTT_PASSWORD': 'env_pass',
        'MQTT_TOPIC': 'env/topic'
    })
    def test_init_from_environment(self):
        """Test initialization from environment variables"""
        client = MQTTPowerClient()
        
        assert client.broker_host == 'env.mqtt.com'
        assert client.broker_port == 8883
        assert client.username == 'env_user'
        assert client.password == 'env_pass'
        assert client.topic == 'env/topic'
    
    @patch.dict(os.environ, {}, clear=True)
    def test_init_missing_broker_host(self):
        """Test initialization fails without broker host"""
        with pytest.raises(ValueError, match="broker host must be provided"):
            MQTTPowerClient(topic="test/topic")
    
    @patch.dict(os.environ, {'MQTT_BROKER_HOST': 'test.com'}, clear=True)
    def test_init_missing_topic(self):
        """Test initialization fails without topic"""
        with pytest.raises(ValueError, match="topic must be provided"):
            MQTTPowerClient()
    
    def test_init_default_port(self):
        """Test that default port is 1883"""
        client = MQTTPowerClient(broker_host="test.com", topic="test/topic")
        assert client.broker_port == 1883
    
    @patch('src.backend.mqtt_client.mqtt.Client')
    def test_init_sets_callbacks(self, mock_mqtt_client):
        """Test that MQTT callbacks are set during initialization"""
        mock_client_instance = Mock()
        mock_mqtt_client.return_value = mock_client_instance
        
        client = MQTTPowerClient(broker_host="test.com", topic="test/topic")
        
        # Verify callbacks were set
        assert mock_client_instance.on_connect is not None
        assert mock_client_instance.on_message is not None
        assert mock_client_instance.on_disconnect is not None
    
    @patch('src.backend.mqtt_client.mqtt.Client')
    def test_init_sets_credentials(self, mock_mqtt_client):
        """Test that credentials are set when provided"""
        mock_client_instance = Mock()
        mock_mqtt_client.return_value = mock_client_instance
        
        client = MQTTPowerClient(
            broker_host="test.com",
            topic="test/topic",
            username="user",
            password="pass"
        )
        
        mock_client_instance.username_pw_set.assert_called_once_with("user", "pass")
    
    @patch.dict('os.environ', {}, clear=True)
    @patch('src.backend.mqtt_client.mqtt.Client')
    def test_init_no_credentials(self, mock_mqtt_client):
        """Test that username_pw_set is not called without credentials"""
        mock_client_instance = Mock()
        mock_mqtt_client.return_value = mock_client_instance
        
        client = MQTTPowerClient(broker_host="test.com", topic="test/topic")
        
        mock_client_instance.username_pw_set.assert_not_called()


class TestMQTTPowerClientConnection:
    """Test connection and disconnection"""
    
    @patch('src.backend.mqtt_client.mqtt.Client')
    def test_connect_success(self, mock_mqtt_client):
        """Test successful connection"""
        mock_client_instance = Mock()
        mock_mqtt_client.return_value = mock_client_instance
        
        client = MQTTPowerClient(broker_host="test.com", topic="test/topic")
        result = client.connect()
        
        assert result is True
        mock_client_instance.connect.assert_called_once_with("test.com", 1883, 60)
        mock_client_instance.loop_start.assert_called_once()
    
    @patch('src.backend.mqtt_client.mqtt.Client')
    def test_connect_failure(self, mock_mqtt_client):
        """Test connection failure"""
        mock_client_instance = Mock()
        mock_client_instance.connect.side_effect = Exception("Connection error")
        mock_mqtt_client.return_value = mock_client_instance
        
        client = MQTTPowerClient(broker_host="test.com", topic="test/topic")
        result = client.connect()
        
        assert result is False
    
    @patch('src.backend.mqtt_client.mqtt.Client')
    def test_disconnect(self, mock_mqtt_client):
        """Test disconnection"""
        mock_client_instance = Mock()
        mock_mqtt_client.return_value = mock_client_instance
        
        client = MQTTPowerClient(broker_host="test.com", topic="test/topic")
        client.is_connected = True
        client.disconnect()
        
        mock_client_instance.loop_stop.assert_called_once()
        mock_client_instance.disconnect.assert_called_once()
        assert client.is_connected is False


class TestMQTTPowerClientCallbacks:
    """Test MQTT callback handlers"""
    
    @patch('src.backend.mqtt_client.mqtt.Client')
    def test_on_connect_success(self, mock_mqtt_client):
        """Test on_connect callback with successful connection"""
        mock_client_instance = Mock()
        mock_mqtt_client.return_value = mock_client_instance
        
        client = MQTTPowerClient(broker_host="test.com", topic="test/topic")
        
        # Simulate successful connection (rc=0)
        client._on_connect(mock_client_instance, None, {}, 0)
        
        assert client.is_connected is True
        mock_client_instance.subscribe.assert_called_once_with("test/topic")
    
    @patch('src.backend.mqtt_client.mqtt.Client')
    def test_on_connect_failure(self, mock_mqtt_client):
        """Test on_connect callback with connection failure"""
        mock_client_instance = Mock()
        mock_mqtt_client.return_value = mock_client_instance
        
        client = MQTTPowerClient(broker_host="test.com", topic="test/topic")
        
        # Simulate failed connection (rc!=0)
        client._on_connect(mock_client_instance, None, {}, 1)
        
        assert client.is_connected is False
        mock_client_instance.subscribe.assert_not_called()
    
    @patch('src.backend.mqtt_client.mqtt.Client')
    def test_on_message_valid_power_value(self, mock_mqtt_client):
        """Test on_message with valid power value"""
        mock_client_instance = Mock()
        mock_mqtt_client.return_value = mock_client_instance
        
        client = MQTTPowerClient(broker_host="test.com", topic="test/topic")
        
        # Create mock MQTT message
        mock_message = Mock()
        mock_message.topic = "test/topic"
        mock_message.payload = b"1250.5"
        
        client._on_message(mock_client_instance, None, mock_message)
        
        assert client.current_power == 1250.5
        assert client.last_updated is not None
    
    @patch('src.backend.mqtt_client.mqtt.Client')
    def test_on_message_with_callback(self, mock_mqtt_client):
        """Test that power callback is called when message received"""
        mock_client_instance = Mock()
        mock_mqtt_client.return_value = mock_client_instance
        
        client = MQTTPowerClient(broker_host="test.com", topic="test/topic")
        
        # Set up callback
        callback_mock = Mock()
        client.set_power_callback(callback_mock)
        
        # Create mock MQTT message
        mock_message = Mock()
        mock_message.topic = "test/topic"
        mock_message.payload = b"950.0"
        
        client._on_message(mock_client_instance, None, mock_message)
        
        callback_mock.assert_called_once_with(950.0)
    
    @patch('src.backend.mqtt_client.mqtt.Client')
    def test_on_message_invalid_value(self, mock_mqtt_client):
        """Test on_message with invalid (non-numeric) value"""
        mock_client_instance = Mock()
        mock_mqtt_client.return_value = mock_client_instance
        
        client = MQTTPowerClient(broker_host="test.com", topic="test/topic")
        
        # Create mock MQTT message with invalid data
        mock_message = Mock()
        mock_message.topic = "test/topic"
        mock_message.payload = b"invalid"
        
        # Should not raise exception, should handle gracefully
        client._on_message(mock_client_instance, None, mock_message)
        
        # Power should remain None
        assert client.current_power is None
    
    @patch('src.backend.mqtt_client.mqtt.Client')
    def test_on_message_zero_power(self, mock_mqtt_client):
        """Test on_message with zero power value"""
        mock_client_instance = Mock()
        mock_mqtt_client.return_value = mock_client_instance
        
        client = MQTTPowerClient(broker_host="test.com", topic="test/topic")
        
        mock_message = Mock()
        mock_message.topic = "test/topic"
        mock_message.payload = b"0.0"
        
        client._on_message(mock_client_instance, None, mock_message)
        
        assert client.current_power == 0.0
    
    @patch('src.backend.mqtt_client.mqtt.Client')
    def test_on_message_negative_power(self, mock_mqtt_client):
        """Test on_message with negative power value (e.g., solar feeding back)"""
        mock_client_instance = Mock()
        mock_mqtt_client.return_value = mock_client_instance
        
        client = MQTTPowerClient(broker_host="test.com", topic="test/topic")
        
        mock_message = Mock()
        mock_message.topic = "test/topic"
        mock_message.payload = b"-500.0"
        
        client._on_message(mock_client_instance, None, mock_message)
        
        assert client.current_power == -500.0
    
    @patch('src.backend.mqtt_client.mqtt.Client')
    def test_on_message_whitespace_handling(self, mock_mqtt_client):
        """Test that whitespace is stripped from message"""
        mock_client_instance = Mock()
        mock_mqtt_client.return_value = mock_client_instance
        
        client = MQTTPowerClient(broker_host="test.com", topic="test/topic")
        
        mock_message = Mock()
        mock_message.topic = "test/topic"
        mock_message.payload = b"  850.5  \n"
        
        client._on_message(mock_client_instance, None, mock_message)
        
        assert client.current_power == 850.5
    
    @patch('src.backend.mqtt_client.mqtt.Client')
    def test_on_disconnect_expected(self, mock_mqtt_client):
        """Test on_disconnect callback for expected disconnection"""
        mock_client_instance = Mock()
        mock_mqtt_client.return_value = mock_client_instance
        
        client = MQTTPowerClient(broker_host="test.com", topic="test/topic")
        client.is_connected = True
        
        # rc=0 means expected disconnection
        client._on_disconnect(mock_client_instance, None, 0)
        
        assert client.is_connected is False
    
    @patch('src.backend.mqtt_client.mqtt.Client')
    def test_on_disconnect_unexpected(self, mock_mqtt_client):
        """Test on_disconnect callback for unexpected disconnection"""
        mock_client_instance = Mock()
        mock_mqtt_client.return_value = mock_client_instance
        
        client = MQTTPowerClient(broker_host="test.com", topic="test/topic")
        client.is_connected = True
        
        # rc!=0 means unexpected disconnection
        client._on_disconnect(mock_client_instance, None, 1)
        
        assert client.is_connected is False


class TestMQTTPowerClientGetters:
    """Test getter methods"""
    
    @patch('src.backend.mqtt_client.mqtt.Client')
    def test_get_current_power_with_data(self, mock_mqtt_client):
        """Test get_current_power when data is available"""
        mock_client_instance = Mock()
        mock_mqtt_client.return_value = mock_client_instance
        
        client = MQTTPowerClient(broker_host="test.com", topic="test/topic")
        client.current_power = 1500.0
        
        assert client.get_current_power() == 1500.0
    
    @patch('src.backend.mqtt_client.mqtt.Client')
    def test_get_current_power_no_data(self, mock_mqtt_client):
        """Test get_current_power when no data available"""
        mock_client_instance = Mock()
        mock_mqtt_client.return_value = mock_client_instance
        
        client = MQTTPowerClient(broker_host="test.com", topic="test/topic")
        
        assert client.get_current_power() is None
    
    @patch('src.backend.mqtt_client.mqtt.Client')
    def test_get_last_updated(self, mock_mqtt_client):
        """Test get_last_updated returns correct timestamp"""
        mock_client_instance = Mock()
        mock_mqtt_client.return_value = mock_client_instance
        
        client = MQTTPowerClient(broker_host="test.com", topic="test/topic")
        
        # Simulate message reception
        test_time = datetime.now()
        client.last_updated = test_time
        
        assert client.get_last_updated() == test_time
    
    @patch('src.backend.mqtt_client.mqtt.Client')
    def test_get_connection_status_connected(self, mock_mqtt_client):
        """Test get_connection_status when connected"""
        mock_client_instance = Mock()
        mock_mqtt_client.return_value = mock_client_instance
        
        client = MQTTPowerClient(broker_host="test.com", topic="test/topic")
        client.is_connected = True
        
        assert client.get_connection_status() is True
    
    @patch('src.backend.mqtt_client.mqtt.Client')
    def test_get_connection_status_disconnected(self, mock_mqtt_client):
        """Test get_connection_status when disconnected"""
        mock_client_instance = Mock()
        mock_mqtt_client.return_value = mock_client_instance
        
        client = MQTTPowerClient(broker_host="test.com", topic="test/topic")
        client.is_connected = False
        
        assert client.get_connection_status() is False


class TestMQTTPowerClientCallbackManagement:
    """Test callback management"""
    
    @patch('src.backend.mqtt_client.mqtt.Client')
    def test_set_power_callback(self, mock_mqtt_client):
        """Test setting power callback"""
        mock_client_instance = Mock()
        mock_mqtt_client.return_value = mock_client_instance
        
        client = MQTTPowerClient(broker_host="test.com", topic="test/topic")
        
        callback = Mock()
        client.set_power_callback(callback)
        
        assert client.power_callback == callback
    
    @patch('src.backend.mqtt_client.mqtt.Client')
    def test_callback_not_called_without_setting(self, mock_mqtt_client):
        """Test that callback is not called if not set"""
        mock_client_instance = Mock()
        mock_mqtt_client.return_value = mock_client_instance
        
        client = MQTTPowerClient(broker_host="test.com", topic="test/topic")
        
        mock_message = Mock()
        mock_message.topic = "test/topic"
        mock_message.payload = b"100.0"
        
        # Should not raise exception even without callback
        client._on_message(mock_client_instance, None, mock_message)
        
        assert client.current_power == 100.0


class TestMQTTPowerClientIntegration:
    """Integration tests"""
    
    @patch('src.backend.mqtt_client.mqtt.Client')
    def test_full_message_flow(self, mock_mqtt_client):
        """Test complete message receiving flow"""
        mock_client_instance = Mock()
        mock_mqtt_client.return_value = mock_client_instance
        
        # Track callback invocations
        received_values = []
        
        def power_callback(power):
            received_values.append(power)
        
        # Create and connect client
        client = MQTTPowerClient(
            broker_host="test.com",
            broker_port=1883,
            topic="power/consumption"
        )
        client.set_power_callback(power_callback)
        
        # Simulate connection
        client._on_connect(mock_client_instance, None, {}, 0)
        assert client.is_connected is True
        
        # Simulate multiple messages
        for power_value in [100.0, 150.0, 200.0]:
            mock_message = Mock()
            mock_message.topic = "power/consumption"
            mock_message.payload = str(power_value).encode()
            client._on_message(mock_client_instance, None, mock_message)
        
        # Verify all values were received
        assert received_values == [100.0, 150.0, 200.0]
        assert client.current_power == 200.0


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
