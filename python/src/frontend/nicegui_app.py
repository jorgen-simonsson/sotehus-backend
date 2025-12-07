#!/usr/bin/env python3
"""
Swedish Electricity Spot Price & Power Monitoring Dashboard
NiceGUI Version

Real-time web dashboard for monitoring Swedish electricity spot prices 
and household power consumption.
"""

from nicegui import ui, app
from datetime import datetime, timezone, timedelta
from typing import Optional, Dict, Any
import asyncio
from asyncio import get_running_loop, new_event_loop, set_event_loop
from src.backend.spotprice import SpotPriceClient
from src.backend.mqtt_client import MQTTPowerClient
from src.backend.solar_edge import SolarEdgeClient, is_sun_up, calculate_solar_update_interval
from src.application.data_manager import DataManager


def get_current_time() -> datetime:
    """Get current time with proper timezone handling (local time)"""
    return datetime.now().astimezone()


def format_timestamp(dt: datetime) -> str:
    """Format datetime to string with timezone awareness"""
    return dt.strftime("%Y-%m-%d %H:%M:%S")

class SpotPriceDashboard:
    """Main dashboard class for managing spot price and power monitoring"""
    
    def __init__(self, 
                 data_manager: DataManager,
                 spot_price_client: Optional[SpotPriceClient] = None,
                 solar_client: Optional[SolarEdgeClient] = None):
        """
        Initialize the dashboard with dependency injection.
        
        Args:
            data_manager: Centralized data manager for shared state
            spot_price_client: Client for fetching spot prices (optional, will create if None)
            solar_client: Client for fetching solar data (optional, will create if None)
        """
        # Injected dependencies
        self.data_manager = data_manager
        self.spot_price_client = spot_price_client or SpotPriceClient()
        self.solar_client = solar_client
        
        # Spot price state
        self.current_price: Optional[float] = None
        self.loading: bool = False
        self.error_message: str = ""
        self.last_updated: str = ""
        self.region: str = "SE4"  # Always use SE4
        
        # MQTT Power consumption state
        self.current_power: Optional[float] = None
        self.mqtt_connected: bool = False
        self.mqtt_error: str = ""
        self.power_last_updated: str = ""
        
        # Solar power state
        self.current_solar_power: Optional[float] = None
        self.solar_error: str = ""
        self.solar_last_updated: str = ""
        self.solar_available: bool = False
        self.solar_update_interval: int = 10  # Will be calculated dynamically
        
        # UI elements (will be set when building UI)
        self.price_label: Optional[ui.label] = None
        self.price_info_label: Optional[ui.label] = None
        self.price_error_label: Optional[ui.label] = None
        self.price_updated_label: Optional[ui.label] = None
        self.price_spinner: Optional[ui.spinner] = None
        
        self.power_label: Optional[ui.label] = None
        self.power_status_label: Optional[ui.label] = None
        self.power_error_label: Optional[ui.label] = None
        self.power_updated_label: Optional[ui.label] = None
        self.power_connecting_container: Optional[ui.element] = None
        self.power_data_container: Optional[ui.element] = None
        
        self.solar_label: Optional[ui.label] = None
        self.solar_status_label: Optional[ui.label] = None
        self.solar_error_label: Optional[ui.label] = None
        self.solar_updated_label: Optional[ui.label] = None
        self.solar_data_container: Optional[ui.element] = None
        
        # Initialize data collection (happens at app startup, runs continuously)
        # These operations are independent of web client connections
        self.setup_mqtt()  # Connects to MQTT broker for real-time power data
        self.fetch_spot_price()  # Fetches initial spot price
        self.check_solar_availability()  # Checks if solar monitoring is configured
        
        # Track the last update time
        self.last_price_update: Optional[datetime] = None  
        self.last_solar_update: Optional[datetime] = None  # Track last solar update
        
        # Calculate optimal solar update interval
        if self.solar_available:
            self.solar_update_interval = calculate_solar_update_interval()

        # Start background updates (runs continuously, independent of web clients)
        self.start_background_updates()
    
    def power_update_callback(self, power: float) -> None:
        """Callback for MQTT power updates."""
        # Update data via data manager
        self.data_manager.update_power_data(power, get_current_time())
        
        # Trigger UI update
        self.update_power_ui()
    
    def setup_mqtt(self):
        """Initialize MQTT connection"""
        try:
            # Get or create MQTT client via data manager
            client = self.data_manager.create_mqtt_client()
            
            if client:
                # Set the callback to update this dashboard instance
                client.set_power_callback(self.power_update_callback)
                
                if client.connect():
                    self.mqtt_connected = True
                    self.mqtt_error = ""
                    print("MQTT connected successfully")
                else:
                    self.mqtt_connected = False
                    self.mqtt_error = "Failed to connect to MQTT broker"
                    print(f"MQTT connection failed: {self.mqtt_error}")
            else:
                self.mqtt_connected = False
                self.mqtt_error = "MQTT client not configured"
        except Exception as e:
            self.mqtt_connected = False
            self.mqtt_error = f"MQTT connection error: {str(e)}"
            print(f"MQTT exception: {e}")
    
    def check_solar_availability(self):
        """Check if SolarEdge configuration is available"""
        try:
            # Try to create solar client if not provided
            if self.solar_client is None:
                self.solar_client = SolarEdgeClient()
            
            self.solar_available = True
            self.solar_error = ""
            print("SolarEdge configuration found")
            # Try to fetch initial data
            self.fetch_solar_power()
        except ValueError as e:
            self.solar_available = False
            self.solar_error = "SolarEdge not configured"
            self.solar_client = None
            print(f"SolarEdge not available: {e}")
        except Exception as e:
            self.solar_available = False
            self.solar_error = f"SolarEdge error: {str(e)}"
            self.solar_client = None
            print(f"SolarEdge exception: {e}")
    
    def fetch_solar_power(self):
        """Fetch the current solar power production"""
        if not self.solar_available or self.solar_client is None:
            return
        
        try:
            power = self.solar_client.get_current_power_production()
            if power is not None:
                self.current_solar_power = round(power, 2)
                self.solar_last_updated = format_timestamp(get_current_time())
                self.last_solar_update = get_current_time()  # Track update time
                self.solar_error = ""
                print(f"Solar power updated: {power}W")
                
                # Update data manager for InfluxDB logging
                self.data_manager.update_solar_production(self.current_solar_power)
            else:
                self.solar_error = "No solar data available"
        except Exception as e:
            self.solar_error = f"Solar error: {str(e)}"
            print(f"Solar fetch error: {e}")
        
        self.update_solar_ui()
    
    def fetch_spot_price(self):
        """Fetch the latest spot price from the API"""
        try:
            self.current_price = self.spot_price_client.get_current_price()
            self.last_price_update = get_current_time()
            self.last_updated = format_timestamp(self.last_price_update)  # Update last_updated
            print(f"Spot price updated: {self.current_price} at {self.last_price_update}")
            
            # Update data manager for InfluxDB logging
            if self.current_price is not None:
                self.data_manager.update_spot_price(self.current_price)
            
            self.update_price_ui()  # Ensure UI is updated
        except Exception as e:
            print(f"Error fetching spot price: {e}")

    def check_and_refresh_spot_price(self):
        """Check if we've crossed a 15-minute boundary and refresh spot price if needed"""
        now = get_current_time()
        current_minute = now.minute
        
        # Check if we're at a 15-minute boundary (0, 15, 30, 45)
        if current_minute % 15 == 0:
            # Only refresh if we haven't updated in the last minute
            if self.last_updated:
                last_update_time = datetime.strptime(self.last_updated, "%Y-%m-%d %H:%M:%S").astimezone()
                time_since_update = (now - last_update_time).total_seconds()
                
                # Refresh if it's been more than 60 seconds
                if time_since_update > 60:
                    print(f"15-minute boundary detected, refreshing spot price")
                    self.fetch_spot_price()
            else:
                # No previous update, fetch now
                self.fetch_spot_price()
    
    def set_region(self, new_region: str):
        """Set the region and fetch new price"""
        self.region = new_region
        self.fetch_spot_price()
    
    def update_price_ui(self):
        """Update the spot price UI elements"""
        if self.price_spinner:
            self.price_spinner.visible = self.loading
        
        if self.price_label:
            if self.current_price is not None:
                self.price_label.text = f"{self.current_price:.2f} SEK/kWh"
                self.price_label.visible = True
            else:
                self.price_label.visible = False
        
        if self.price_info_label:
            # Hide the info label to save space on mobile
            self.price_info_label.visible = False
        
        if self.price_error_label:
            self.price_error_label.text = self.error_message
            self.price_error_label.visible = bool(self.error_message)
        
        if self.price_updated_label:
            self.price_updated_label.text = f"Last updated: {self.last_updated}" if self.last_updated else ""
            self.price_updated_label.visible = bool(self.last_updated)
    
    def update_power_ui(self):
        """Update the power consumption UI elements"""
        # Read latest data from data manager
        latest_data = self.data_manager.get_latest_power_data()
        
        if latest_data:
            self.current_power = latest_data['power']
            self.power_last_updated = format_timestamp(latest_data['timestamp'])
            self.mqtt_error = ""
        
        # Update UI elements
        if self.power_connecting_container:
            self.power_connecting_container.visible = not self.mqtt_connected
        
        if self.power_data_container:
            self.power_data_container.visible = self.mqtt_connected
        
        if self.power_label and self.current_power is not None:
            self.power_label.text = f"{self.current_power:.0f} W"
        
        if self.power_status_label:
            self.power_status_label.visible = self.mqtt_connected
        
        if self.power_error_label:
            self.power_error_label.text = f"⚠️ {self.mqtt_error}" if self.mqtt_error else ""
            self.power_error_label.visible = bool(self.mqtt_error)
        
        if self.power_updated_label:
            self.power_updated_label.text = f"Last updated: {self.power_last_updated}" if self.power_last_updated else ""
            self.power_updated_label.visible = bool(self.power_last_updated)
    
    def update_solar_ui(self):
        """Update the solar power UI elements"""
        if not self.solar_available:
            if self.solar_data_container:
                self.solar_data_container.visible = False
            return
        
        if self.solar_data_container:
            self.solar_data_container.visible = True
        
        if self.solar_label and self.current_solar_power is not None:
            # Display in W if less than 1000W, otherwise in kW
            if self.current_solar_power < 1000:
                self.solar_label.text = f"{self.current_solar_power:.1f} W"
                print(f"DEBUG: Updated solar UI label to {self.current_solar_power:.1f} W")
            else:
                power_kw = self.current_solar_power / 1000
                self.solar_label.text = f"{power_kw:.2f} kW"
                print(f"DEBUG: Updated solar UI label to {power_kw:.2f} kW")
        elif self.solar_label is None:
            print("DEBUG: solar_label is None")
        elif self.current_solar_power is None:
            print(f"DEBUG: current_solar_power is None")
        
        if self.solar_status_label:
            if self.current_solar_power is not None:
                if self.current_solar_power > 0:
                    self.solar_status_label.text = "☀️ Producing power"
                else:
                    self.solar_status_label.text = "🌙 No production"
            else:
                self.solar_status_label.text = "❓ Status unknown"
        
        if self.solar_error_label:
            self.solar_error_label.text = f"⚠️ {self.solar_error}" if self.solar_error else ""
            self.solar_error_label.visible = bool(self.solar_error)
        
        if self.solar_updated_label:
            self.solar_updated_label.text = f"Last updated: {self.solar_last_updated}" if self.solar_last_updated else ""
            self.solar_updated_label.visible = bool(self.solar_last_updated)
    
    def start_background_updates(self):
        """Start the background task for periodic updates."""
        try:
            loop = get_running_loop()
        except RuntimeError:
            loop = new_event_loop()
            set_event_loop(loop)
        loop.create_task(self.background_update_loop())
    
    async def background_update_loop(self):
        """Background task to update spot price and solar power periodically"""
        last_interval_update = None
        
        while True:
            await asyncio.sleep(60)  # Check every 1 minute
            
            # Get connected clients count from data manager
            has_clients = self.data_manager.has_connected_clients()
            
            # Recalculate solar update interval daily (at midnight or on first run)
            now = get_current_time()
            if last_interval_update is None or now.date() != last_interval_update.date():
                if self.solar_available:
                    self.solar_update_interval = calculate_solar_update_interval()
                    last_interval_update = now
            
            # Update solar power with optimizations:
            # 1. Only if solar is available
            # 2. Only if sun is up
            # 3. Only if clients are connected
            # 4. Using adaptive interval based on API limits
            if self.solar_available and has_clients and is_sun_up():
                if self.last_solar_update is None:
                    # First update
                    self.fetch_solar_power()
                else:
                    minutes_since_update = (now - self.last_solar_update).total_seconds() / 60
                    if minutes_since_update >= self.solar_update_interval:
                        self.fetch_solar_power()
            
            # Update spot price when crossing 15-minute boundaries (0, 15, 30, 45)
            current_quarter = now.minute // 15  # 0, 1, 2, or 3
            
            if self.last_price_update is None:
                # First update
                self.fetch_spot_price()
            else:
                last_quarter = self.last_price_update.minute // 15
                # Check if we've crossed into a new 15-minute period
                if current_quarter != last_quarter or now.hour != self.last_price_update.hour:
                    self.fetch_spot_price()

    def build_ui(self):
        """Build the user interface"""
        with ui.column().classes('w-full items-center p-8 gap-4'):
            # Header
            ui.label('Sotehus').classes('text-6xl font-bold')
            
            # Spot Price Section
            with ui.card().classes('w-full max-w-lg p-6'):
                with ui.column().classes('items-center gap-4'):
                    self.price_spinner = ui.spinner(size='lg')
                    self.price_spinner.visible = self.loading
                    
                    with ui.column().classes('items-center gap-1'):
                        self.price_label = ui.label().classes('text-4xl font-bold')
                        self.price_info_label = ui.label().classes('text-base')
                    
                    self.price_error_label = ui.label().classes('text-red-600')
                    self.price_updated_label = ui.label().classes('text-sm text-gray-600')
            
            # Power Consumption Section
            with ui.card().classes('w-full max-w-lg p-6 mt-4'):
                with ui.column().classes('items-center gap-3'):
                    ui.label('🏠 Grid Consumption').classes('text-2xl font-semibold mb-4')
                    
                    # Connecting state
                    with ui.column().classes('items-center gap-2') as self.power_connecting_container:
                        ui.label('Connecting to power monitoring...')
                        ui.spinner(size='md')
                    
                    # Connected state
                    with ui.column().classes('items-center gap-1') as self.power_data_container:
                        self.power_label = ui.label().classes('text-3xl font-bold text-orange-600')
                    
                    # Status labels
                    self.power_status_label = ui.label('🟢 MQTT Connected').classes('text-sm text-green-600')
                    self.power_error_label = ui.label('🟢 MQTT Connected').classes('text-sm text-red-600')
                    self.power_updated_label = ui.label('🟢 MQTT Connected').classes('text-sm text-gray-600')
            
            # Solar Power Section
            with ui.card().classes('w-full max-w-lg p-6 mt-4'):
                with ui.column().classes('items-center gap-3'):
                    ui.label('☀️ Solar Production').classes('text-2xl font-semibold mb-4')
                    
                    # Solar data display
                    with ui.column().classes('items-center gap-1') as self.solar_data_container:
                        self.solar_label = ui.label().classes('text-3xl font-bold text-yellow-600')
                    
                    # Status labels
                    self.solar_status_label = ui.label().classes('text-sm text-gray-600')
                    self.solar_error_label = ui.label().classes('text-sm text-red-600')
                    self.solar_updated_label = ui.label().classes('text-sm text-gray-600')
            
            # Version footer
            version = self._read_version()
            ui.label(f'v{version}').classes('text-xs text-gray-400 mt-8')
        
        # Initial UI update
        self.update_price_ui()
        self.update_power_ui()
        self.update_solar_ui()
        
        # Start background updates using timers
        self.start_background_updates()
    
    def _read_version(self) -> str:
        """Read version from version.txt file"""
        try:
            from pathlib import Path
            import os
            # Try multiple possible locations for version.txt
            possible_paths = [
                Path(__file__).parent.parent.parent / 'version.txt',  # Relative to this file
                Path('/app/version.txt'),  # Docker container location
                Path(os.getcwd()) / 'version.txt',  # Current working directory
            ]
            
            for version_file in possible_paths:
                if version_file.exists():
                    version = version_file.read_text().strip()
                    return version
        except Exception as e:
            print(f"Could not read version: {e}")
        return "unknown"


# ============================================================================
# Application Initialization (runs at startup, independent of web clients)
# ============================================================================

# Initialize data manager (singleton)
data_manager = DataManager()

# Initialize InfluxDB client for continuous data logging
# This runs independently of web client connections
try:
    influxdb_client = data_manager.create_influxdb_client()
    if influxdb_client and influxdb_client.is_connected():
        print("InfluxDB integration enabled - continuous logging active")
    else:
        print("InfluxDB integration disabled (not configured or connection failed)")
except Exception as e:
    print(f"InfluxDB initialization skipped: {e}")

# Create the dashboard instance with dependency injection
# This sets up MQTT connection and background tasks that run continuously
# regardless of whether any web clients are connected
dashboard = SpotPriceDashboard(data_manager=data_manager)

@ui.page('/')
async def index():
    """Main page with client tracking"""
    # Increment connected clients counter via data manager
    data_manager.increment_clients()
    
    # Build the UI
    dashboard.build_ui()
    
    # Register cleanup on client disconnect
    async def on_disconnect():
        data_manager.decrement_clients()
    
    app.on_disconnect(on_disconnect)

if __name__ in {"__main__", "__mp_main__"}:
    # Run the NiceGUI app
    ui.run(
        title='Sotehus',
        reload=False,
        host='0.0.0.0',
        port=8080
    )
