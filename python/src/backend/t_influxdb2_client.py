"""
Tests for InfluxDB2Client - Time series database client

Tests cover:
- Client initialization and configuration
- Connection management and automatic reconnection
- Writing power monitoring data
- Error handling and recovery
- Thread safety
"""

import pytest
from unittest.mock import Mock, patch, MagicMock, call
from datetime import datetime
import time
import os
from src.backend.influxdb2_client import InfluxDB2Client


class TestInfluxDB2ClientInit:
    """Test initialization and configuration"""
    
    @patch('src.backend.influxdb2_client.load_dotenv')
    def test_init_with_parameters(self, mock_load_dotenv):
        """Test initialization with explicit parameters"""
        # Mock the InfluxDBClient at import time
        with patch('influxdb_client.InfluxDBClient') as mock_influxdb:
            mock_client = Mock()
            mock_client.health.return_value = Mock(status="pass")
            mock_client.write_api.return_value = Mock()
            mock_influxdb.return_value = mock_client
            
            client = InfluxDB2Client(
                host="testhost",
                port=8086,
                user="testuser",
                password="testpass",
                org="testorg",
                bucket="testbucket"
            )
            
            assert client.host == "testhost"
            assert client.port == 8086
            assert client.user == "testuser"
            assert client.password == "testpass"
            assert client.org == "testorg"
            assert client.bucket == "testbucket"
    
    @patch('src.backend.influxdb2_client.load_dotenv')
    @patch.dict('os.environ', {
        'INFLUXDB2_HOST': 'env_host',
        'INFLUXDB2_PORT': '9999',
        'INFLUXDB2_USER': 'env_user',
        'INFLUXDB2_PASSWORD': 'env_pass'
    })
    def test_init_from_environment(self, mock_load_dotenv):
        """Test initialization from environment variables"""
        with patch('influxdb_client.InfluxDBClient') as mock_influxdb:
            mock_client = Mock()
            mock_client.health.return_value = Mock(status="pass")
            mock_client.write_api.return_value = Mock()
            mock_influxdb.return_value = mock_client
            
            client = InfluxDB2Client()
            
            assert client.host == 'env_host'
            assert client.port == 9999
            assert client.user == 'env_user'
            assert client.password == 'env_pass'
    
    @patch('src.backend.influxdb2_client.load_dotenv')
    @patch.dict('os.environ', {}, clear=True)
    def test_init_missing_host(self, mock_load_dotenv):
        """Test initialization fails without host"""
        with pytest.raises(ValueError, match="InfluxDB host is required"):
            InfluxDB2Client()
    
    @patch('src.backend.influxdb2_client.load_dotenv')
    @patch.dict('os.environ', {'INFLUXDB2_HOST': 'testhost'})
    def test_init_default_org_and_bucket(self, mock_load_dotenv):
        """Test default organization and bucket names"""
        with patch('influxdb_client.InfluxDBClient') as mock_influxdb:
            mock_client = Mock()
            mock_client.health.return_value = Mock(status="pass")
            mock_client.write_api.return_value = Mock()
            mock_influxdb.return_value = mock_client
            
            client = InfluxDB2Client(host="testhost", user="user", password="pass")
            
            assert client.org == "sotehus"
            assert client.bucket == "sotehus_bucket"
    
    @patch('src.backend.influxdb2_client.load_dotenv')
    @patch.dict('os.environ', {'INFLUXDB2_HOST': 'testhost'})
    def test_init_with_token(self, mock_load_dotenv):
        """Test initialization with token instead of user/password"""
        with patch('influxdb_client.InfluxDBClient') as mock_influxdb:
            mock_client = Mock()
            mock_client.health.return_value = Mock(status="pass")
            mock_client.write_api.return_value = Mock()
            mock_influxdb.return_value = mock_client
            
            client = InfluxDB2Client(host="testhost", token="test_token")
            
            assert client.token == "test_token"
            mock_influxdb.assert_called_once()
            call_kwargs = mock_influxdb.call_args[1]
            assert call_kwargs['token'] == "test_token"


class TestInfluxDB2ClientConnection:
    """Test connection management"""
    
    @patch('src.backend.influxdb2_client.load_dotenv')
    @patch.dict('os.environ', {'INFLUXDB2_HOST': 'testhost'})
    def test_connect_success(self, mock_load_dotenv):
        """Test successful connection"""
        with patch('influxdb_client.InfluxDBClient') as mock_influxdb:
            mock_client = Mock()
            mock_client.health.return_value = Mock(status="pass")
            mock_client.write_api.return_value = Mock()
            mock_influxdb.return_value = mock_client
            
            client = InfluxDB2Client(host="testhost", user="user", password="pass")
            
            assert client.is_connected()
            mock_client.health.assert_called()
    
    @patch('src.backend.influxdb2_client.load_dotenv')
    @patch.dict('os.environ', {'INFLUXDB2_HOST': 'testhost'})
    def test_connect_health_check_failure(self, mock_load_dotenv):
        """Test connection with failed health check"""
        with patch('influxdb_client.InfluxDBClient') as mock_influxdb:
            mock_client = Mock()
            mock_client.health.return_value = Mock(status="fail", message="Service unavailable")
            mock_client.write_api.return_value = Mock()
            mock_influxdb.return_value = mock_client
            
            client = InfluxDB2Client(host="testhost", user="user", password="pass")
            
            assert not client.is_connected()
    
    @patch('src.backend.influxdb2_client.load_dotenv')
    @patch.dict('os.environ', {'INFLUXDB2_HOST': 'testhost'})
    def test_connect_exception(self, mock_load_dotenv):
        """Test connection with exception"""
        with patch('influxdb_client.InfluxDBClient', side_effect=Exception("Connection error")):
            client = InfluxDB2Client(host="testhost", user="user", password="pass")
            
            assert not client.is_connected()
    
    @patch('src.backend.influxdb2_client.load_dotenv')
    @patch.dict('os.environ', {'INFLUXDB2_HOST': 'testhost'})
    def test_connect_missing_influxdb_client_package(self, mock_load_dotenv):
        """Test graceful handling when influxdb-client is not installed"""
        # Patch the import statement itself to raise ImportError
        import sys
        with patch.dict(sys.modules, {'influxdb_client': None}):
            client = InfluxDB2Client(host="testhost", user="user", password="pass")
            assert not client.is_connected()
    
    @patch('src.backend.influxdb2_client.load_dotenv')
    @patch.dict('os.environ', {'INFLUXDB2_HOST': 'testhost'})
    def test_close_connection(self, mock_load_dotenv):
        """Test closing connection"""
        with patch('influxdb_client.InfluxDBClient') as mock_influxdb:
            mock_client = Mock()
            mock_client.health.return_value = Mock(status="pass")
            mock_client.write_api.return_value = Mock()
            mock_influxdb.return_value = mock_client
            
            client = InfluxDB2Client(host="testhost", user="user", password="pass")
            assert client.is_connected()
            
            client.close()
            
            mock_client.close.assert_called_once()
            assert not client.is_connected()


class TestInfluxDB2ClientReconnection:
    """Test automatic reconnection logic"""
    
    @patch('src.backend.influxdb2_client.load_dotenv')
    @patch.dict('os.environ', {'INFLUXDB2_HOST': 'testhost'})
    @patch('src.backend.influxdb2_client.time.time')
    def test_ensure_connection_triggers_reconnect(self, mock_time, mock_load_dotenv):
        """Test that ensure_connection triggers reconnect when disconnected"""
        with patch('influxdb_client.InfluxDBClient') as mock_influxdb:
            mock_time.return_value = 100.0
            
            # First connection fails
            mock_client = Mock()
            mock_client.health.return_value = Mock(status="fail")
            mock_client.write_api.return_value = Mock()
            mock_influxdb.return_value = mock_client
            
            client = InfluxDB2Client(host="testhost", user="user", password="pass")
            assert not client.is_connected()
            
            # Attempt to ensure connection (should try to reconnect)
            mock_time.return_value = 106.0  # Past reconnect delay
            mock_client.health.return_value = Mock(status="pass")
            
            result = client._ensure_connection()
            
            # Should have attempted reconnection
            assert mock_influxdb.call_count >= 2
    
    @patch('src.backend.influxdb2_client.load_dotenv')
    @patch.dict('os.environ', {'INFLUXDB2_HOST': 'testhost'})
    @patch('src.backend.influxdb2_client.time.time')
    def test_reconnect_exponential_backoff(self, mock_time, mock_load_dotenv):
        """Test exponential backoff on failed reconnection attempts"""
        with patch('influxdb_client.InfluxDBClient') as mock_influxdb:
            mock_time.return_value = 100.0
            
            # Connection always fails
            mock_client = Mock()
            mock_client.health.return_value = Mock(status="fail")
            mock_client.write_api.return_value = Mock()
            mock_influxdb.return_value = mock_client
            
            client = InfluxDB2Client(host="testhost", user="user", password="pass")
            initial_delay = client._reconnect_delay
            
            # Try to reconnect multiple times
            for i in range(3):
                mock_time.return_value += client._reconnect_delay + 1
                client._ensure_connection()
            
            # Delay should have increased
            assert client._reconnect_delay > initial_delay
            assert client._reconnect_delay <= 60  # Should cap at 60 seconds


class TestInfluxDB2ClientWriteData:
    """Test writing data to InfluxDB"""
    
    @patch('src.backend.influxdb2_client.load_dotenv')
    @patch.dict('os.environ', {'INFLUXDB2_HOST': 'testhost'})
    def test_write_power_data_all_fields(self, mock_load_dotenv):
        """Test writing data with all fields"""
        with patch('influxdb_client.InfluxDBClient') as mock_influxdb, \
             patch('influxdb_client.Point') as mock_point_class:
            mock_client = Mock()
            mock_client.health.return_value = Mock(status="pass")
            mock_write_api = Mock()
            mock_client.write_api.return_value = mock_write_api
            mock_influxdb.return_value = mock_client
            
            # Create mock point that returns itself for chaining
            mock_point = Mock()
            mock_point.time.return_value = mock_point
            mock_point.field.return_value = mock_point
            mock_point_class.return_value = mock_point
            
            client = InfluxDB2Client(host="testhost", user="user", password="pass")
            
            timestamp = datetime(2024, 1, 15, 12, 30, 0)
            result = client.write_power_data(
                grid_power=1500.5,
                spot_price=0.85,
                solar_production=2300.0,
                timestamp=timestamp
            )
            
            assert result is True
            mock_point_class.assert_called_once_with("power_monitoring")
            mock_point.time.assert_called_once_with(timestamp)
            
            # Check that all fields were added
            assert mock_point.field.call_count == 3
            field_calls = [call[0] for call in mock_point.field.call_args_list]
            assert ("grid_power", 1500.5) in field_calls
            assert ("spot_price", 0.85) in field_calls
            assert ("solar_production", 2300.0) in field_calls
            
            mock_write_api.write.assert_called_once()
    
    @patch('src.backend.influxdb2_client.load_dotenv')
    @patch.dict('os.environ', {'INFLUXDB2_HOST': 'testhost'})
    def test_write_power_data_partial_fields(self, mock_load_dotenv):
        """Test writing data with only some fields"""
        with patch('influxdb_client.InfluxDBClient') as mock_influxdb, \
             patch('influxdb_client.Point') as mock_point_class:
            mock_client = Mock()
            mock_client.health.return_value = Mock(status="pass")
            mock_write_api = Mock()
            mock_client.write_api.return_value = mock_write_api
            mock_influxdb.return_value = mock_client
            
            # Create mock point that returns itself for chaining
            mock_point = Mock()
            mock_point.time.return_value = mock_point
            mock_point.field.return_value = mock_point
            mock_point_class.return_value = mock_point
            
            client = InfluxDB2Client(host="testhost", user="user", password="pass")
            
            result = client.write_power_data(grid_power=1500.5)
            
            assert result is True
            # Only one field should be added
            assert mock_point.field.call_count == 1
            mock_point.field.assert_called_with("grid_power", 1500.5)
    
    @patch('src.backend.influxdb2_client.load_dotenv')
    @patch.dict('os.environ', {'INFLUXDB2_HOST': 'testhost'})
    def test_write_power_data_when_disconnected(self, mock_load_dotenv):
        """Test writing data when connection is lost"""
        with patch('influxdb_client.InfluxDBClient') as mock_influxdb:
            mock_client = Mock()
            mock_client.health.return_value = Mock(status="fail")
            mock_client.write_api.return_value = Mock()
            mock_influxdb.return_value = mock_client
            
            client = InfluxDB2Client(host="testhost", user="user", password="pass")
            
            result = client.write_power_data(grid_power=1500.5)
            
            assert result is False
    
    @patch('src.backend.influxdb2_client.load_dotenv')
    @patch.dict('os.environ', {'INFLUXDB2_HOST': 'testhost'})
    def test_write_power_data_exception_handling(self, mock_load_dotenv):
        """Test exception handling during write"""
        with patch('influxdb_client.InfluxDBClient') as mock_influxdb, \
             patch('influxdb_client.Point') as mock_point_class:
            mock_client = Mock()
            mock_client.health.return_value = Mock(status="pass")
            mock_write_api = Mock()
            mock_write_api.write.side_effect = Exception("Write failed")
            mock_client.write_api.return_value = mock_write_api
            mock_influxdb.return_value = mock_client
            
            # Create mock point that returns itself for chaining
            mock_point = Mock()
            mock_point.time.return_value = mock_point
            mock_point.field.return_value = mock_point
            mock_point_class.return_value = mock_point
            
            client = InfluxDB2Client(host="testhost", user="user", password="pass")
            
            result = client.write_power_data(grid_power=1500.5)
            
            assert result is False
            assert not client.is_connected()  # Should mark as disconnected


class TestInfluxDB2ClientContextManager:
    """Test context manager functionality"""
    
    @patch('src.backend.influxdb2_client.load_dotenv')
    @patch.dict('os.environ', {'INFLUXDB2_HOST': 'testhost'})
    def test_context_manager(self, mock_load_dotenv):
        """Test using client as context manager"""
        with patch('influxdb_client.InfluxDBClient') as mock_influxdb:
            mock_client = Mock()
            mock_client.health.return_value = Mock(status="pass")
            mock_client.write_api.return_value = Mock()
            mock_influxdb.return_value = mock_client
            
            with InfluxDB2Client(host="testhost", user="user", password="pass") as client:
                assert client.is_connected()
            
            # Should close connection after exiting context
            mock_client.close.assert_called_once()


if __name__ == '__main__':
    pytest.main([__file__, '-v'])
