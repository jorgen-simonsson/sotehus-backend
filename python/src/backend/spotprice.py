
import requests
import json
from datetime import datetime
from dateutil import parser
from typing import List, Dict, Any, Optional

class SpotPriceClient:
    """
    A minimal client for fetching current electricity spot prices from elprisetjustnu.se
    """
    
    def __init__(self, base_url: str = "https://www.elprisetjustnu.se/api/v1/prices") -> None:
        """
        Initialize the SpotPriceClient
        
        Args:
            base_url: Base URL for the API endpoint
        """
        self.base_url: str = base_url
    
    def get_spot_prices(self, region: str = "SE4") -> Optional[List[Dict[str, Any]]]:
        """
        Get current spot prices from elprisetjustnu.se API
        
        Args:
            region: The region code (default: SE4)
        
        Returns:
            List of dictionaries containing spot price data, or None if error
        """
        # Get current date
        current_date: datetime = datetime.now()
        year: str = current_date.strftime("%Y")
        month_day: str = current_date.strftime("%m-%d")
        
        # Construct the API URL
        url: str = f"{self.base_url}/{year}/{month_day}_{region}.json"
        
        try:
            # Make the API request
            response: requests.Response = requests.get(url)
            response.raise_for_status()  # Raise an exception for bad status codes
            
            # Parse JSON string into dictionary
            data: List[Dict[str, Any]] = json.loads(response.text)
            
            return data
            
        except requests.exceptions.RequestException as e:
            print(f"Error making API request: {e}")
            return None
        except json.JSONDecodeError as e:
            print(f"Error parsing JSON response: {e}")
            return None

    def get_current_spot_price(self, price_data: List[Dict[str, Any]]) -> Optional[float]:
        """
        Get the current spot price in SEK based on system time
        
        Args:
            price_data: List of price dictionaries from the API
        
        Returns:
            Current spot price in SEK per kWh, or None if not found
        """
        if not price_data:
            print("No price data provided")
            return None
        
        # Get current system time with timezone awareness
        current_time: datetime = datetime.now().astimezone()
        
        # Find the price entry that covers the current time
        for entry in price_data:
            try:
                # Parse the time strings to datetime objects
                time_start: datetime = parser.parse(entry['time_start'])
                time_end: datetime = parser.parse(entry['time_end'])
                
                # Convert current time to the same timezone as the data
                current_time_tz: datetime = current_time.astimezone(time_start.tzinfo)
                
                # Check if current time falls within this price period
                if time_start <= current_time_tz < time_end:
                    return float(entry['SEK_per_kWh'])
                    
            except (KeyError, ValueError, TypeError) as e:
                print(f"Error parsing entry: {e}")
                continue
        
        print("No matching price found for current time")
        return None

    def get_current_price(self, region: str = "SE4") -> Optional[float]:
        """
        Convenience method to get current spot price in one call
        
        Args:
            region: The region code (default: SE4)
            
        Returns:
            Current spot price in SEK per kWh, or None if error
        """
        data: Optional[List[Dict[str, Any]]] = self.get_spot_prices(region)
        if data:
            return self.get_current_spot_price(data)
        return None


# Example usage
if __name__ == "__main__":
    # Create client instance
    client: SpotPriceClient = SpotPriceClient()
    
    # Get current spot price directly
    current_price: Optional[float] = client.get_current_price()
    if current_price is not None:
        print(f"Current spot price: {current_price:.5f} SEK per kWh")
    else:
        print("Could not determine current spot price")
