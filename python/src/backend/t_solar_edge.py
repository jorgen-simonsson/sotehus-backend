"""
Tests for SolarEdgeClient and solar-related utility functions

Tests cover:
- SolarEdge API client initialization and configuration
- Current power production fetching
- Sun position calculations (is_sun_up)
- Solar update interval calculations
- Error handling and edge cases
"""

import pytest
from unittest.mock import Mock, patch, MagicMock
from datetime import datetime, timezone, timedelta
import requests
import json
import os
from src.backend.solar_edge import (
    SolarEdgeClient, 
    is_sun_up, 
    calculate_solar_update_interval
)


class TestSolarEdgeClient:
    """Test suite for SolarEdgeClient"""
    
    def test_init_with_parameters(self):
        """Test initialization with explicit parameters"""
        api_key = "test_key_123"
        site_id = "test_site_456"
        
        client = SolarEdgeClient(api_key=api_key, site_id=site_id)
        
        assert client.api_key == api_key
        assert client.site_id == site_id
        assert client.base_url == "https://monitoringapi.solaredge.com"
    
    def test_init_custom_base_url(self):
        """Test initialization with custom base URL"""
        custom_url = "https://custom.solaredge.com"
        
        client = SolarEdgeClient(
            api_key="key",
            site_id="site",
            base_url=custom_url
        )
        
        assert client.base_url == custom_url
    
    @patch.dict(os.environ, {'SOLAREDGE_API_KEY': 'env_key', 'SOLAREDGE_SITE_ID': 'env_site'})
    def test_init_from_environment(self):
        """Test initialization from environment variables"""
        client = SolarEdgeClient()
        
        assert client.api_key == 'env_key'
        assert client.site_id == 'env_site'
    
    @patch('src.backend.solar_edge.load_dotenv')
    @patch.dict(os.environ, {}, clear=True)
    def test_init_missing_api_key(self, mock_load_dotenv):
        """Test initialization fails without API key"""
        with pytest.raises(ValueError, match="API key is required"):
            SolarEdgeClient()
    
    @patch('src.backend.solar_edge.load_dotenv')
    @patch.dict(os.environ, {'SOLAREDGE_API_KEY': 'key'}, clear=True)
    def test_init_missing_site_id(self, mock_load_dotenv):
        """Test initialization fails without site ID"""
        with pytest.raises(ValueError, match="Site ID is required"):
            SolarEdgeClient()
    
    @patch('src.backend.solar_edge.requests.get')
    def test_make_request_success(self, mock_get):
        """Test successful API request"""
        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.return_value = {"data": "test"}
        mock_get.return_value = mock_response
        
        client = SolarEdgeClient(api_key="key", site_id="site")
        result = client._make_request("/test/endpoint")
        
        assert result == {"data": "test"}
        mock_get.assert_called_once()
    
    @patch('src.backend.solar_edge.requests.get')
    def test_make_request_includes_api_key(self, mock_get):
        """Test that API key is included in request parameters"""
        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.return_value = {}
        mock_get.return_value = mock_response
        
        client = SolarEdgeClient(api_key="secret_key", site_id="site")
        client._make_request("/test")
        
        # Verify API key was added to params
        call_args = mock_get.call_args
        assert call_args[1]['params']['api_key'] == 'secret_key'
    
    @patch('src.backend.solar_edge.requests.get')
    def test_make_request_network_error(self, mock_get):
        """Test handling of network errors"""
        mock_get.side_effect = requests.exceptions.RequestException("Network error")
        
        client = SolarEdgeClient(api_key="key", site_id="site")
        result = client._make_request("/test")
        
        assert result is None
    
    @patch('src.backend.solar_edge.requests.get')
    def test_make_request_http_error(self, mock_get):
        """Test handling of HTTP errors (429, 500, etc.)"""
        mock_response = Mock()
        mock_response.raise_for_status.side_effect = requests.exceptions.HTTPError("429 Too Many Requests")
        mock_get.return_value = mock_response
        
        client = SolarEdgeClient(api_key="key", site_id="site")
        result = client._make_request("/test")
        
        assert result is None
    
    @patch('src.backend.solar_edge.requests.get')
    def test_make_request_invalid_json(self, mock_get):
        """Test handling of invalid JSON response"""
        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.side_effect = json.JSONDecodeError("Invalid JSON", "", 0)
        mock_get.return_value = mock_response
        
        client = SolarEdgeClient(api_key="key", site_id="site")
        result = client._make_request("/test")
        
        assert result is None
    
    @patch('src.backend.solar_edge.requests.get')
    def test_get_current_power_flow_success(self, mock_get):
        """Test successful power flow retrieval"""
        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "siteCurrentPowerFlow": {
                "PV": {"currentPower": 5.5}
            }
        }
        mock_get.return_value = mock_response
        
        client = SolarEdgeClient(api_key="key", site_id="12345")
        result = client.get_current_power_flow()
        
        assert result is not None
        assert "siteCurrentPowerFlow" in result
    
    @patch('src.backend.solar_edge.requests.get')
    def test_get_current_power_production_success(self, mock_get):
        """Test successful power production retrieval"""
        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "siteCurrentPowerFlow": {
                "PV": {"currentPower": 3.5}  # 3.5 kW
            }
        }
        mock_get.return_value = mock_response
        
        client = SolarEdgeClient(api_key="key", site_id="site")
        result = client.get_current_power_production()
        
        # Should convert kW to W
        assert result == 3500.0
    
    @patch('src.backend.solar_edge.requests.get')
    def test_get_current_power_production_zero(self, mock_get):
        """Test handling of zero power production (nighttime)"""
        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "siteCurrentPowerFlow": {
                "PV": {"currentPower": 0}
            }
        }
        mock_get.return_value = mock_response
        
        client = SolarEdgeClient(api_key="key", site_id="site")
        result = client.get_current_power_production()
        
        assert result == 0.0
    
    @patch('src.backend.solar_edge.requests.get')
    def test_get_current_power_production_api_failure(self, mock_get):
        """Test handling when API call fails"""
        mock_get.side_effect = requests.exceptions.RequestException("API error")
        
        client = SolarEdgeClient(api_key="key", site_id="site")
        result = client.get_current_power_production()
        
        assert result is None
    
    @patch('src.backend.solar_edge.requests.get')
    def test_get_current_power_production_missing_data(self, mock_get):
        """Test handling of missing PV data in response"""
        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "siteCurrentPowerFlow": {}  # Missing PV key
        }
        mock_get.return_value = mock_response
        
        client = SolarEdgeClient(api_key="key", site_id="site")
        result = client.get_current_power_production()
        
        assert result is None
    
    @patch('src.backend.solar_edge.requests.get')
    def test_get_current_power_production_malformed_response(self, mock_get):
        """Test handling of completely malformed response"""
        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.return_value = "unexpected string"
        mock_get.return_value = mock_response
        
        client = SolarEdgeClient(api_key="key", site_id="site")
        result = client.get_current_power_production()
        
        assert result is None
    
    @patch('src.backend.solar_edge.requests.get')
    def test_get_current_power_production_invalid_power_value(self, mock_get):
        """Test handling of invalid power value"""
        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "siteCurrentPowerFlow": {
                "PV": {"currentPower": "invalid"}
            }
        }
        mock_get.return_value = mock_response
        
        client = SolarEdgeClient(api_key="key", site_id="site")
        result = client.get_current_power_production()
        
        assert result is None


class TestIsSunUp:
    """Test suite for is_sun_up() function"""
    
    @patch('src.backend.solar_edge.datetime')
    @patch('src.backend.solar_edge.sun')
    def test_sun_up_during_day(self, mock_sun, mock_datetime):
        """Test that function returns True during daylight hours"""
        # Set time to noon
        mock_now = datetime(2025, 6, 15, 12, 0, tzinfo=timezone.utc)
        mock_datetime.now.return_value = mock_now
        
        # Mock sun times
        mock_sun.return_value = {
            'sunrise': datetime(2025, 6, 15, 4, 30, tzinfo=timezone.utc),
            'sunset': datetime(2025, 6, 15, 20, 30, tzinfo=timezone.utc)
        }
        
        result = is_sun_up()
        assert result is True
    
    @patch('src.backend.solar_edge.datetime')
    @patch('src.backend.solar_edge.sun')
    def test_sun_down_at_night(self, mock_sun, mock_datetime):
        """Test that function returns False during nighttime"""
        # Set time to midnight
        mock_now = datetime(2025, 6, 15, 0, 0, tzinfo=timezone.utc)
        mock_datetime.now.return_value = mock_now
        
        # Mock sun times
        mock_sun.return_value = {
            'sunrise': datetime(2025, 6, 15, 4, 30, tzinfo=timezone.utc),
            'sunset': datetime(2025, 6, 15, 20, 30, tzinfo=timezone.utc)
        }
        
        result = is_sun_up()
        assert result is False
    
    @patch('src.backend.solar_edge.datetime')
    @patch('src.backend.solar_edge.sun')
    def test_sun_at_sunrise(self, mock_sun, mock_datetime):
        """Test at exact sunrise time"""
        sunrise_time = datetime(2025, 6, 15, 4, 30, tzinfo=timezone.utc)
        mock_datetime.now.return_value = sunrise_time
        
        mock_sun.return_value = {
            'sunrise': sunrise_time,
            'sunset': datetime(2025, 6, 15, 20, 30, tzinfo=timezone.utc)
        }
        
        result = is_sun_up()
        # Should be True at sunrise (inclusive)
        assert result is True
    
    @patch('src.backend.solar_edge.datetime')
    @patch('src.backend.solar_edge.sun')
    def test_sun_at_sunset(self, mock_sun, mock_datetime):
        """Test at exact sunset time"""
        sunset_time = datetime(2025, 6, 15, 20, 30, tzinfo=timezone.utc)
        mock_datetime.now.return_value = sunset_time
        
        mock_sun.return_value = {
            'sunrise': datetime(2025, 6, 15, 4, 30, tzinfo=timezone.utc),
            'sunset': sunset_time
        }
        
        result = is_sun_up()
        # Should be True at sunset (inclusive)
        assert result is True
    
    @patch('src.backend.solar_edge.datetime')
    @patch('src.backend.solar_edge.sun')
    def test_fallback_on_calculation_error(self, mock_sun, mock_datetime):
        """Test fallback to daytime hours when calculation fails"""
        mock_datetime.now.return_value = datetime(2025, 6, 15, 14, 0, tzinfo=timezone.utc)
        mock_sun.side_effect = Exception("Calculation error")
        
        # Should fall back to hour-based check (6 AM - 8 PM)
        result = is_sun_up()
        assert isinstance(result, bool)


class TestCalculateSolarUpdateInterval:
    """Test suite for calculate_solar_update_interval() function"""
    
    @patch('src.backend.solar_edge.sun')
    @patch('src.backend.solar_edge.datetime')
    def test_calculate_interval_summer(self, mock_datetime, mock_sun):
        """Test interval calculation for long summer day"""
        mock_datetime.now.return_value = datetime(2025, 6, 15, 12, 0, tzinfo=timezone.utc)
        
        # Summer in Stockholm: ~18 hours of daylight
        mock_sun.return_value = {
            'sunrise': datetime(2025, 6, 15, 2, 30, tzinfo=timezone.utc),  # ~3:30 AM local
            'sunset': datetime(2025, 6, 15, 20, 30, tzinfo=timezone.utc)   # ~9:30 PM local
        }
        
        # 18 hours = 1080 minutes
        # 300 calls * 0.9 = 270 calls
        # 1080 / 270 = 4 minutes -> adjusted to 5 (minimum)
        result = calculate_solar_update_interval(max_daily_calls=300, usage_percent=0.9)
        
        assert result >= 5  # Should respect minimum
        assert isinstance(result, int)
    
    @patch('src.backend.solar_edge.sun')
    @patch('src.backend.solar_edge.datetime')
    def test_calculate_interval_winter(self, mock_datetime, mock_sun):
        """Test interval calculation for short winter day"""
        mock_datetime.now.return_value = datetime(2025, 12, 15, 12, 0, tzinfo=timezone.utc)
        
        # Winter in Stockholm: ~6 hours of daylight
        mock_sun.return_value = {
            'sunrise': datetime(2025, 12, 15, 7, 30, tzinfo=timezone.utc),   # ~8:30 AM local
            'sunset': datetime(2025, 12, 15, 13, 30, tzinfo=timezone.utc)    # ~2:30 PM local
        }
        
        # 6 hours = 360 minutes
        # 300 calls * 0.9 = 270 calls
        # 360 / 270 = 1.3 minutes -> adjusted to 5 (minimum)
        result = calculate_solar_update_interval(max_daily_calls=300, usage_percent=0.9)
        
        assert result >= 5
        assert isinstance(result, int)
    
    def test_calculate_interval_custom_parameters(self):
        """Test with custom max calls and usage percent"""
        with patch('src.backend.solar_edge.sun') as mock_sun:
            with patch('src.backend.solar_edge.datetime') as mock_datetime:
                mock_datetime.now.return_value = datetime(2025, 6, 15, 12, 0, tzinfo=timezone.utc)
                
                # 12 hours of daylight = 720 minutes
                mock_sun.return_value = {
                    'sunrise': datetime(2025, 6, 15, 6, 0, tzinfo=timezone.utc),
                    'sunset': datetime(2025, 6, 15, 18, 0, tzinfo=timezone.utc)
                }
                
                # 500 calls * 0.8 = 400 calls
                # 720 / 400 = 1.8 minutes -> adjusted to 5 (minimum)
                result = calculate_solar_update_interval(max_daily_calls=500, usage_percent=0.8)
                
                assert result >= 5
    
    @patch('src.backend.solar_edge.sun')
    @patch('src.backend.solar_edge.datetime')
    def test_calculate_interval_respects_minimum(self, mock_datetime, mock_sun):
        """Test that interval never goes below minimum (5 minutes)"""
        mock_datetime.now.return_value = datetime(2025, 6, 15, 12, 0, tzinfo=timezone.utc)
        
        # Very long day: 20 hours
        mock_sun.return_value = {
            'sunrise': datetime(2025, 6, 15, 2, 0, tzinfo=timezone.utc),
            'sunset': datetime(2025, 6, 15, 22, 0, tzinfo=timezone.utc)
        }
        
        # Even with many calls allowed, should respect minimum
        result = calculate_solar_update_interval(max_daily_calls=1000, usage_percent=0.95)
        
        assert result >= 5
    
    @patch('src.backend.solar_edge.sun')
    def test_calculate_interval_error_fallback(self, mock_sun):
        """Test fallback to default when calculation fails"""
        mock_sun.side_effect = Exception("Calculation error")
        
        result = calculate_solar_update_interval()
        
        # Should return default 10 minutes on error
        assert result == 10
    
    @patch('src.backend.solar_edge.sun')
    @patch('src.backend.solar_edge.datetime')
    def test_calculate_interval_low_api_limit(self, mock_datetime, mock_sun):
        """Test with very low API call limit"""
        mock_datetime.now.return_value = datetime(2025, 6, 15, 12, 0, tzinfo=timezone.utc)
        
        # 10 hours daylight = 600 minutes
        mock_sun.return_value = {
            'sunrise': datetime(2025, 6, 15, 7, 0, tzinfo=timezone.utc),
            'sunset': datetime(2025, 6, 15, 17, 0, tzinfo=timezone.utc)
        }
        
        # Only 50 calls per day * 0.9 = 45 calls
        # 600 / 45 = 13.3 minutes
        result = calculate_solar_update_interval(max_daily_calls=50, usage_percent=0.9)
        
        assert result >= 13


class TestSolarEdgeIntegration:
    """Integration tests combining multiple components"""
    
    @patch.dict(os.environ, {'SOLAREDGE_API_KEY': 'test_key', 'SOLAREDGE_SITE_ID': 'test_site'})
    @patch('src.backend.solar_edge.requests.get')
    def test_full_power_production_flow(self, mock_get):
        """Test complete flow from client creation to power retrieval"""
        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "siteCurrentPowerFlow": {
                "PV": {"currentPower": 4.2}
            }
        }
        mock_get.return_value = mock_response
        
        # Create client (from environment)
        client = SolarEdgeClient()
        
        # Get power production
        power = client.get_current_power_production()
        
        assert power == 4200.0  # 4.2 kW = 4200 W
        assert client.api_key == 'test_key'
        assert client.site_id == 'test_site'


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
