# ⚡ Swedish Electricity Spot Price & Power Monitoring Dashboard

A real-time web dashboard for monitoring Swedish electricity spot prices and household power consumption. Built with [NiceGUI](https://nicegui.io/) - a Python-based web framework that's easy to use and deploy.

![Python](https://img.shields.io/badge/Python-3.12-blue)
![NiceGUI](https://img.shields.io/badge/NiceGUI-1.4+-purple)
![License](https://img.shields.io/badge/License-MIT-green)

## 🌟 Features

- **Real-Time Spot Prices**: Live electricity spot prices for all Swedish regions (SE1-SE4)
- **Power Consumption Monitoring**: Real-time power usage via MQTT integration
- **Auto-Updating Interface**: UI refreshes every 3 seconds with latest data
- **Modern UI**: Clean, responsive design using Tailwind CSS
- **Easy Deployment**: Simple Python application, no complex build steps
- **Region Selection**: Easy switching between Swedish electricity regions

## 🚀 Quick Start

### Prerequisites

- Python 3.12 or higher
- MQTT broker (optional, for power monitoring)
- Internet connection (for spot price data)

### Installation

1. **Navigate to project root**:
   ```bash
   cd /path/to/spot
   ```

2. **Create virtual environment** (if not already done):
   ```bash
   python -m venv .venv
   source .venv/bin/activate  # On Windows: .venv\Scripts\activate
   ```

3. **Install dependencies**:
   ```bash
   pip install -r requirements.txt
   ```

4. **Configure MQTT** (optional):
   ```bash
   # Copy .env from root if not already done
   cp .env.example .env
   # Edit .env with your MQTT broker details
   ```

5. **Run the application**:
   ```bash
   # From project root:
   python run_nicegui.py
   
   # OR directly:
   cd spot_nicegui
   python nicegui_app.py
   ```

6. **Open in browser**:
   ```
   http://localhost:8080
   ```

## 📊 Data Sources

See main [README.md](../README.md) for complete details:

- **Spot Prices**: [elprisetjustnu.se](https://www.elprisetjustnu.se/) API (15-minute intervals)
- **Power Data**: MQTT broker (Home Assistant, Zigbee2MQTT, etc.)

## 🖥️ UI Features

### Modern Design
- **Tailwind CSS**: Responsive, modern styling
- **Card-based Layout**: Clean, organized sections
- **Real-time Updates**: Smooth UI updates every 3 seconds
- **Loading States**: Visual feedback during data fetch

### Sections
1. **Spot Price Display**
   - Large price display with SEK/kWh
   - Region dropdown selector
   - Loading spinner
   - Last updated timestamp

2. **Power Consumption**
   - Real-time wattage display
   - Connection status indicator
   - Auto-reconnect handling
   - Error messages

3. **Information**
   - About section
   - Region descriptions

## 🔧 Configuration

### Port Configuration

Change the port in `nicegui_app.py`:

```python
ui.run(
    title='⚡ Spotpris Dashboard',
    reload=False,
    host='0.0.0.0',
    port=8080  # Change this
)
```

### Update Frequency

Modify the sleep duration in the `background_update_loop` method:

```python
async def background_update_loop(self):
    while True:
        await asyncio.sleep(3)  # Change to 5, 10, etc.
        self.update_power_ui()
```

## 📁 Project Structure

```
spot_frontend/
├── nicegui_app.py        # Main NiceGUI application
└── README.md            # This file

# Backend modules (in parent directory):
../backend/
├── spotprice.py         # Spot price API client
└── mqtt_client.py       # MQTT power monitoring client
```

For complete project structure, see main [README.md](../../README.md).

## ️ Architecture

### Data Flow

```
┌─────────────────────┐
│   Spot Price API    │
│ (elprisetjustnu.se) │
└──────────┬──────────┘
           │ HTTP GET
           ↓
┌─────────────────────┐      ┌──────────────────┐
│   SpotPriceClient   │      │  MQTT Broker     │
│  (src/backend)      │      │                  │
└──────────┬──────────┘      └────────┬─────────┘
           │                          │ Subscribe
           │                          ↓
           │                 ┌──────────────────┐
           │                 │  MQTTPowerClient │
           │                 │  (src/backend)   │
           │                 └────────┬─────────┘
           │                          │
           ↓                          ↓
┌─────────────────────────────────────────────┐
│          Global Shared State                │
│  (_latest_power_data, spot price cache)    │
└──────────────────┬──────────────────────────┘
                   │
                   ↓
┌─────────────────────────────────────────────┐
│      Background Update Task                 │
│   (asyncio.sleep(3) + update_power_ui())    │
└──────────────────┬──────────────────────────┘
                   │ Updates UI elements
                   ↓
┌─────────────────────────────────────────────┐
│         Browser (NiceGUI Frontend)          │
│         (WebSocket connection)              │
└─────────────────────────────────────────────┘
```

### Key Architecture Features

1. **Single Dashboard Instance**: One dashboard object shared by all clients
2. **Background Task**: Single asyncio task updates the UI periodically
3. **Direct UI Updates**: Modify UI element properties directly (no state layer)
4. **Simple and Efficient**: Straightforward architecture with minimal overhead

## 🐛 Troubleshooting

### Port Already in Use

If port 8080 is already in use:

```bash
# Find process using port
lsof -i :8080

# Kill the process or change port in nicegui_app.py
```

### Module Import Errors

Make sure you're in the correct directory:

```bash
cd spot_nicegui
python nicegui_app.py  # Run from this directory
```

### MQTT Connection Issues

See main [README.md](../README.md) troubleshooting section for detailed MQTT debugging steps.

## 🚢 Deployment

### Local Network Access

The app runs on `0.0.0.0:8080` by default, making it accessible from other devices on your network:

```
http://your-local-ip:8080
```

### Production Deployment

For production, consider:

1. **Reverse Proxy**: Use nginx or Caddy
2. **HTTPS**: Add SSL certificate
3. **Process Manager**: Use systemd or supervisor
4. **Docker**: Containerize the application

Example systemd service:

```ini
[Unit]
Description=Spotpris NiceGUI Dashboard
After=network.target

[Service]
Type=simple
User=your-user
WorkingDirectory=/path/to/spot_nicegui
ExecStart=/path/to/.venv/bin/python nicegui_app.py
Restart=always

[Install]
WantedBy=multi-user.target
```

## 🤝 Contributing

Contributions welcome! Improvements to shared modules (`spotprice.py`, `mqtt_client.py`) benefit the entire project.

## 📄 License

MIT License - same as parent project

## 🙏 Acknowledgments

- **NiceGUI Team** - For the excellent Python web framework
- **elprisetjustnu.se** - For providing free Swedish electricity spot price API
- **Nord Pool** - Source of spot price data

---

**Built with ❤️ in Sweden** 🇸🇪
