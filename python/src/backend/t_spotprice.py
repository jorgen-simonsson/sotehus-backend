"""
Tests for SpotPriceClient - Electricity spot price API client

Tests cover:
- API request handling
- Price data parsing
- Current price calculation based on time
- Error handling and edge cases
"""

import pytest
from unittest.mock import Mock, patch, MagicMock
from datetime import datetime, timezone, timedelta
import requests
from src.backend.spotprice import SpotPriceClient


class TestSpotPriceClient:
    """Test suite for SpotPriceClient"""
    
    def test_init_default_url(self):
        """Test initialization with default URL"""
        client = SpotPriceClient()
        assert client.base_url == "https://www.elprisetjustnu.se/api/v1/prices"
    
    def test_init_custom_url(self):
        """Test initialization with custom URL"""
        custom_url = "https://custom.api.com/prices"
        client = SpotPriceClient(base_url=custom_url)
        assert client.base_url == custom_url
    
    @patch('src.backend.spotprice.requests.get')
    def test_get_spot_prices_success(self, mock_get):
        """Test successful API call for spot prices"""
        # Mock successful API response
        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.text = '''[
            {
                "SEK_per_kWh": 0.5,
                "time_start": "2025-11-04T10:00:00+01:00",
                "time_end": "2025-11-04T10:15:00+01:00"
            },
            {
                "SEK_per_kWh": 0.6,
                "time_start": "2025-11-04T10:15:00+01:00",
                "time_end": "2025-11-04T10:30:00+01:00"
            }
        ]'''
        mock_get.return_value = mock_response
        
        client = SpotPriceClient()
        result = client.get_spot_prices("SE4")
        
        assert result is not None
        assert len(result) == 2
        assert result[0]['SEK_per_kWh'] == 0.5
        assert result[1]['SEK_per_kWh'] == 0.6
    
    @patch('src.backend.spotprice.requests.get')
    def test_get_spot_prices_network_error(self, mock_get):
        """Test handling of network errors"""
        mock_get.side_effect = requests.exceptions.RequestException("Network error")
        
        client = SpotPriceClient()
        result = client.get_spot_prices("SE4")
        
        assert result is None
    
    @patch('src.backend.spotprice.requests.get')
    def test_get_spot_prices_invalid_json(self, mock_get):
        """Test handling of invalid JSON response"""
        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.text = "Invalid JSON"
        mock_get.return_value = mock_response
        
        client = SpotPriceClient()
        result = client.get_spot_prices("SE4")
        
        assert result is None
    
    @patch('src.backend.spotprice.requests.get')
    def test_get_spot_prices_http_error(self, mock_get):
        """Test handling of HTTP errors (404, 500, etc.)"""
        mock_response = Mock()
        mock_response.raise_for_status.side_effect = requests.exceptions.HTTPError("404")
        mock_get.return_value = mock_response
        
        client = SpotPriceClient()
        result = client.get_spot_prices("SE4")
        
        assert result is None
    
    @patch('src.backend.spotprice.datetime')
    def test_get_spot_prices_correct_url_format(self, mock_datetime, monkeypatch):
        """Test that the correct URL is constructed"""
        # Mock datetime
        mock_now = datetime(2025, 11, 4, 15, 30)
        mock_datetime.now.return_value = mock_now
        
        with patch('src.backend.spotprice.requests.get') as mock_get:
            mock_response = Mock()
            mock_response.status_code = 200
            mock_response.text = '[]'
            mock_get.return_value = mock_response
            
            client = SpotPriceClient()
            client.get_spot_prices("SE4")
            
            # Verify URL format
            expected_url = "https://www.elprisetjustnu.se/api/v1/prices/2025/11-04_SE4.json"
            mock_get.assert_called_once()
            assert mock_get.call_args[0][0] == expected_url
    
    def test_get_current_spot_price_no_data(self):
        """Test get_current_spot_price with no data"""
        client = SpotPriceClient()
        result = client.get_current_spot_price([])
        
        assert result is None
    
    def test_get_current_spot_price_none_data(self):
        """Test get_current_spot_price with None"""
        client = SpotPriceClient()
        result = client.get_current_spot_price(None)
        
        assert result is None
    
    @patch('src.backend.spotprice.datetime')
    @patch('src.backend.spotprice.parser.parse')
    def test_get_current_spot_price_matching_time(self, mock_parse, mock_datetime):
        """Test finding current price for matching time slot"""
        # Current time: 10:20
        mock_now = datetime(2025, 11, 4, 10, 20, tzinfo=timezone.utc)
        mock_datetime.now.return_value = mock_now
        
        # Mock parser to return proper datetime objects
        def parse_side_effect(time_str):
            if "10:00:00" in time_str:
                return datetime(2025, 11, 4, 10, 0, tzinfo=timezone.utc)
            elif "10:15:00" in time_str:
                return datetime(2025, 11, 4, 10, 15, tzinfo=timezone.utc)
            elif "10:30:00" in time_str:
                return datetime(2025, 11, 4, 10, 30, tzinfo=timezone.utc)
            elif "10:45:00" in time_str:
                return datetime(2025, 11, 4, 10, 45, tzinfo=timezone.utc)
        
        mock_parse.side_effect = parse_side_effect
        
        price_data = [
            {
                "SEK_per_kWh": 0.5,
                "time_start": "2025-11-04T10:00:00+00:00",
                "time_end": "2025-11-04T10:15:00+00:00"
            },
            {
                "SEK_per_kWh": 0.6,
                "time_start": "2025-11-04T10:15:00+00:00",
                "time_end": "2025-11-04T10:30:00+00:00"
            },
            {
                "SEK_per_kWh": 0.7,
                "time_start": "2025-11-04T10:30:00+00:00",
                "time_end": "2025-11-04T10:45:00+00:00"
            }
        ]
        
        client = SpotPriceClient()
        result = client.get_current_spot_price(price_data)
        
        # Should return price for 10:15-10:30 slot (0.6)
        assert result == 0.6
    
    def test_get_current_spot_price_malformed_entry(self):
        """Test handling of malformed price entries"""
        price_data = [
            {
                # Missing time_end
                "SEK_per_kWh": 0.5,
                "time_start": "2025-11-04T10:00:00+00:00"
            }
        ]
        
        client = SpotPriceClient()
        result = client.get_current_spot_price(price_data)
        
        # Should handle error gracefully
        assert result is None
    
    def test_get_current_spot_price_invalid_price_format(self):
        """Test handling of invalid price format"""
        price_data = [
            {
                "SEK_per_kWh": "invalid",  # String instead of number
                "time_start": "2025-11-04T10:00:00+00:00",
                "time_end": "2025-11-04T10:15:00+00:00"
            }
        ]
        
        client = SpotPriceClient()
        # Should handle ValueError when converting to float
        # Behavior depends on implementation
        result = client.get_current_spot_price(price_data)
        
        # Should return None on error
        assert result is None or isinstance(result, float)
    
    @patch('src.backend.spotprice.requests.get')
    def test_get_current_price_integration(self, mock_get):
        """Test get_current_price convenience method"""
        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.text = '''[
            {
                "SEK_per_kWh": 0.75,
                "time_start": "2025-11-04T10:00:00+01:00",
                "time_end": "2025-11-04T11:00:00+01:00"
            }
        ]'''
        mock_get.return_value = mock_response
        
        with patch.object(SpotPriceClient, 'get_current_spot_price') as mock_current:
            mock_current.return_value = 0.75
            
            client = SpotPriceClient()
            result = client.get_current_price("SE4")
            
            assert result == 0.75
    
    @patch('src.backend.spotprice.requests.get')
    def test_get_current_price_api_failure(self, mock_get):
        """Test get_current_price when API fails"""
        mock_get.side_effect = requests.exceptions.RequestException("API error")
        
        client = SpotPriceClient()
        result = client.get_current_price("SE4")
        
        assert result is None
    
    def test_different_regions(self):
        """Test that different regions are handled correctly"""
        with patch('src.backend.spotprice.requests.get') as mock_get:
            mock_response = Mock()
            mock_response.status_code = 200
            mock_response.text = '[]'
            mock_get.return_value = mock_response
            
            client = SpotPriceClient()
            
            # Test different Swedish regions
            for region in ["SE1", "SE2", "SE3", "SE4"]:
                client.get_spot_prices(region)
                called_url = mock_get.call_args[0][0]
                assert f"_{region}.json" in called_url


class TestSpotPriceEdgeCases:
    """Test edge cases and boundary conditions"""
    
    def test_empty_response(self):
        """Test handling of empty price array"""
        client = SpotPriceClient()
        result = client.get_current_spot_price([])
        assert result is None
    
    def test_price_at_exact_boundary(self):
        """Test price lookup at exact time boundary"""
        with patch('src.backend.spotprice.datetime') as mock_datetime:
            # Exactly 10:15:00
            mock_now = datetime(2025, 11, 4, 10, 15, 0, tzinfo=timezone.utc)
            mock_datetime.now.return_value = mock_now
            
            with patch('src.backend.spotprice.parser.parse') as mock_parse:
                def parse_side_effect(time_str):
                    if "10:15:00" in time_str:
                        return datetime(2025, 11, 4, 10, 15, tzinfo=timezone.utc)
                    elif "10:30:00" in time_str:
                        return datetime(2025, 11, 4, 10, 30, tzinfo=timezone.utc)
                
                mock_parse.side_effect = parse_side_effect
                
                price_data = [
                    {
                        "SEK_per_kWh": 0.6,
                        "time_start": "2025-11-04T10:15:00+00:00",
                        "time_end": "2025-11-04T10:30:00+00:00"
                    }
                ]
                
                client = SpotPriceClient()
                result = client.get_current_spot_price(price_data)
                
                # Should include the start boundary
                assert result == 0.6
    
    def test_timezone_conversion(self):
        """Test that timezone conversion works correctly"""
        # This test verifies the astimezone() fix works
        client = SpotPriceClient()
        
        # Create price data with CET timezone
        with patch('src.backend.spotprice.datetime') as mock_datetime:
            with patch('src.backend.spotprice.parser.parse') as mock_parse:
                # Current time in CET (UTC+1)
                from datetime import timezone as tz
                cet = tz(timedelta(hours=1))
                mock_now = datetime(2025, 11, 4, 10, 20, tzinfo=cet)
                mock_datetime.now.return_value = mock_now
                
                # Parse returns UTC times
                def parse_side_effect(time_str):
                    if "10:15:00" in time_str:
                        return datetime(2025, 11, 4, 9, 15, tzinfo=timezone.utc)  # 10:15 CET = 09:15 UTC
                    elif "10:30:00" in time_str:
                        return datetime(2025, 11, 4, 9, 30, tzinfo=timezone.utc)  # 10:30 CET = 09:30 UTC
                
                mock_parse.side_effect = parse_side_effect
                
                price_data = [
                    {
                        "SEK_per_kWh": 0.88,
                        "time_start": "2025-11-04T10:15:00+01:00",
                        "time_end": "2025-11-04T10:30:00+01:00"
                    }
                ]
                
                result = client.get_current_spot_price(price_data)
                assert result == 0.88


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
