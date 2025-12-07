import requests
import json
import os
from typing import Dict, Any, Optional
from dotenv import load_dotenv
from datetime import datetime
from astral import LocationInfo
from astral.sun import sun


def is_sun_up() -> bool:
    """
    Check if the sun is currently up in Sweden (SE4 region - Stockholm area)
    Returns True if it's daytime, False otherwise
    """
    try:
        # Stockholm coordinates (representative for SE4)
        location = LocationInfo("Stockholm", "Sweden", "Europe/Stockholm", 59.3293, 18.0686)
        
        # Get today's sun times
        now = datetime.now().astimezone()
        s = sun(location.observer, date=now.date(), tzinfo=now.tzinfo)
        
        sunrise = s['sunrise']
        sunset = s['sunset']
        
        # Check if current time is between sunrise and sunset
        return sunrise <= now <= sunset
    except Exception as e:
        print(f"Error calculating sun position: {e}")
        # Default to daytime hours (6 AM - 8 PM) as fallback
        hour = datetime.now().astimezone().hour
        return 6 <= hour <= 20


def calculate_solar_update_interval(max_daily_calls: int = 300, usage_percent: float = 0.9) -> int:
    """
    Calculate the optimal interval between solar API calls during daylight hours.
    
    Args:
        max_daily_calls: Maximum API calls allowed per day (default: 300)
        usage_percent: Percentage of available calls to use (default: 0.9 for 90%)
    
    Returns:
        Interval in minutes between API calls
    """
    try:
        # Stockholm coordinates
        location = LocationInfo("Stockholm", "Sweden", "Europe/Stockholm", 59.3293, 18.0686)
        
        # Get today's sun times
        now = datetime.now().astimezone()
        s = sun(location.observer, date=now.date(), tzinfo=now.tzinfo)
        
        sunrise = s['sunrise']
        sunset = s['sunset']
        
        # Calculate daylight duration in minutes
        daylight_minutes = (sunset - sunrise).total_seconds() / 60
        
        # Calculate allowed calls (90% of max)
        allowed_calls = int(max_daily_calls * usage_percent)
        
        # Calculate interval in minutes
        interval_minutes = daylight_minutes / allowed_calls
        
        # Ensure minimum interval of 5 minutes to be safe
        interval_minutes = max(5, interval_minutes)
        
        print(f"Solar update interval calculated: {interval_minutes:.1f} minutes "
              f"({allowed_calls} calls over {daylight_minutes/60:.1f} hours of daylight)")
        
        return int(interval_minutes)
        
    except Exception as e:
        print(f"Error calculating update interval: {e}")
        # Default to 10 minutes as fallback
        return 10


class SolarEdgeClient:
    """
    A simple client for fetching current solar panel power production from the SolarEdge monitoring API
    """
    
    def __init__(self, api_key: str = None, site_id: str = None, base_url: str = "https://monitoringapi.solaredge.com") -> None:
        """
        Initialize the SolarEdgeClient
        
        Args:
            api_key: SolarEdge API key (if None, will try to load from environment)
            site_id: SolarEdge site ID (if None, will try to load from environment)
            base_url: Base URL for the SolarEdge API
        """
        # Load environment variables
        load_dotenv()
        
        self.api_key: str = api_key or os.getenv('SOLAREDGE_API_KEY')
        self.site_id: str = site_id or os.getenv('SOLAREDGE_SITE_ID')
        self.base_url: str = base_url
        
        if not self.api_key:
            raise ValueError("SolarEdge API key is required. Set SOLAREDGE_API_KEY in .env file or pass as parameter.")
        
        if not self.site_id:
            raise ValueError("SolarEdge Site ID is required. Set SOLAREDGE_SITE_ID in .env file or pass as parameter.")
    
    def _make_request(self, endpoint: str, params: Dict[str, Any] = None) -> Optional[Dict[str, Any]]:
        """
        Make a request to the SolarEdge API
        
        Args:
            endpoint: API endpoint to call
            params: Additional parameters for the request
            
        Returns:
            JSON response as dictionary, or None if error
        """
        if params is None:
            params = {}
        
        # Add API key to parameters
        params['api_key'] = self.api_key
        
        url = f"{self.base_url}{endpoint}"
        
        try:
            response: requests.Response = requests.get(url, params=params)
            response.raise_for_status()
            
            data: Dict[str, Any] = response.json()
            return data
            
        except requests.exceptions.RequestException as e:
            print(f"Error making API request to {url}: {e}")
            return None
        except json.JSONDecodeError as e:
            print(f"Error parsing JSON response: {e}")
            return None
    
    def get_current_power_flow(self) -> Optional[Dict[str, Any]]:
        """
        Get current power flow data (current production, consumption, etc.)
        
        Returns:
            Dictionary containing current power flow data, or None if error
        """
        endpoint = f"/site/{self.site_id}/currentPowerFlow"
        return self._make_request(endpoint)
    
    def get_current_power_production(self) -> Optional[float]:
        """
        Get the current power production in Watts from PV array
        
        Returns:
            Current power production in Watts, or None if error/no production
        """
        power_flow = self.get_current_power_flow()
        
        if not power_flow:
            return None
        
        try:
            # Navigate through the JSON structure to get PV production
            site_current_power_flow = power_flow.get('siteCurrentPowerFlow', {})
            pv_data = site_current_power_flow.get('PV')
            
            # If PV data is missing, return None instead of 0
            if pv_data is None:
                return None
            
            current_power_kw = pv_data.get('currentPower', 0)
            
            # Convert from kW to W (API returns power in kW)
            current_power_w = float(current_power_kw) * 1000
            
            return current_power_w
            
        except (KeyError, ValueError, TypeError, AttributeError) as e:
            print(f"Error parsing power production data: {e}")
            return None


# Example usage and testing
if __name__ == "__main__":
    try:
        # Create client instance
        print("Initializing SolarEdge client...")
        client = SolarEdgeClient()
        
        print(f"Using Site ID: {client.site_id}")
        print(f"API Key configured: {'Yes' if client.api_key else 'No'}")
        print("-" * 50)
        
        # Test: Get current power production
        print("Testing current power production...")
        current_power = client.get_current_power_production()
        if current_power is not None:
            print(f"Current power production: {current_power:.2f} W ({current_power/1000:.2f} kW)")
        else:
            print("Could not retrieve current power production")
            
        print("\n" + "=" * 50)
        print("SolarEdge API test completed!")
        
    except ValueError as e:
        print(f"Configuration error: {e}")
        print("\nPlease ensure you have set the following in your .env file:")
        print("SOLAREDGE_API_KEY=your_api_key_here")
        print("SOLAREDGE_SITE_ID=your_site_id_here")
    except Exception as e:
        print(f"Unexpected error: {e}")