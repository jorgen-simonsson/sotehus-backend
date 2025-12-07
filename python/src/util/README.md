# Utility Scripts

This directory contains utility scripts for managing and inspecting the sotehus-power application.

## Available Utilities

### listinflux.py

Query and display the most recent records from InfluxDB.

**Purpose**: Inspect time-series data being logged to InfluxDB, showing grid power consumption, spot prices, and solar production.

**Usage**:
```bash
# List last 50 records (default)
python src/util/listinflux.py

# List last 100 records
python src/util/listinflux.py --count 100

# Show help
python src/util/listinflux.py --help
```

**Output Format**:
```
Timestamp                    Grid Power (W)  Spot Price (SEK/kWh)    Solar Production (W)
==========================================================================================================
2025-11-08 14:30:00               1580.5                   0.8523                  2300.0
2025-11-08 14:29:00               1575.2                   0.8523                  2295.5
...

--- Summary Statistics ---
Grid Power:        avg= 1577.8 W, min= 1500.0 W, max= 1650.0 W
Spot Price:        avg= 0.8523 SEK/kWh, min= 0.8200 SEK/kWh, max= 0.9100 SEK/kWh
Solar Production:  avg= 2297.7 W, min= 2200.0 W, max= 2400.0 W
```

**Features**:
- Displays timestamp and all data fields
- Shows summary statistics (average, min, max)
- Handles missing values gracefully
- Configurable record count
- Validates connection before querying

**Requirements**:
- InfluxDB must be configured and running
- Environment variables set in `.env` file
- `influxdb-client` Python package installed

**Configuration** (via `.env`):
```bash
INFLUXDB2_HOST=localhost
INFLUXDB2_PORT=8086
INFLUXDB2_USER=your_username
INFLUXDB2_PASSWORD=your_password
# OR use token authentication:
INFLUXDB2_TOKEN=your_api_token

# Optional:
INFLUXDB2_ORG=sotehus
INFLUXDB2_BUCKET=sotehus_bucket
```

**Troubleshooting**:

*"Error: influxdb-client not installed"*
```bash
pip install influxdb-client
```

*"Error: INFLUXDB2_HOST not set in environment"*
```bash
# Create .env file with your InfluxDB configuration
cp .env-example .env
# Edit .env with your settings
```

*"No records found in the database"*
- Verify the application is running and writing to InfluxDB
- Check that MQTT is connected and sending power data
- Confirm InfluxDB is receiving data: check the InfluxDB UI or logs

## Adding New Utilities

When creating new utility scripts in this directory:

1. **Use clear, descriptive names**: `verb_noun.py` format (e.g., `export_data.py`)
2. **Include docstrings**: Explain purpose, usage, and requirements
3. **Add argument parsing**: Use `argparse` for command-line options
4. **Handle errors gracefully**: Provide helpful error messages
5. **Update this README**: Document the new utility
6. **Make executable**: `chmod +x src/util/your_script.py`

**Template structure**:
```python
#!/usr/bin/env python3
"""
Brief description of utility

Detailed explanation...

Usage:
    python src/util/your_script.py [options]
"""

import argparse
from pathlib import Path

def main():
    parser = argparse.ArgumentParser(description='...')
    # Add arguments
    args = parser.parse_args()
    # Implementation
    
if __name__ == '__main__':
    main()
```
