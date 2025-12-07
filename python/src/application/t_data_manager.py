"""
Tests for DataManager - Centralized state management

This test module verifies:
- Singleton pattern implementation
- Thread safety for all operations
- MQTT client management
- Power data management
- Client connection tracking
"""

import unittest
from unittest.mock import Mock, patch, MagicMock
from datetime import datetime
import threading
import time
from src.application.data_manager import DataManager
from src.backend.mqtt_client import MQTTPowerClient


class TestDataManagerSingleton(unittest.TestCase):
    """Test the singleton pattern implementation"""
    
    def setUp(self):
        """Reset singleton before each test"""
        DataManager._instance = None
    
    def test_singleton_same_instance(self):
        """Test that multiple instantiations return the same instance"""
        dm1 = DataManager()
        dm2 = DataManager()
        self.assertIs(dm1, dm2)
    
    def test_singleton_initialization_once(self):
        """Test that initialization only happens once"""
        dm1 = DataManager()
        initial_id = id(dm1)
        
        dm2 = DataManager()
        self.assertEqual(id(dm2), initial_id)
        
        # Check that internal state is shared
        dm1._test_value = "shared"
        self.assertEqual(dm2._test_value, "shared")
    
    def test_singleton_thread_safety(self):
        """Test that singleton creation is thread-safe"""
        instances = []
        
        def create_instance():
            instance = DataManager()
            instances.append(instance)
        
        threads = [threading.Thread(target=create_instance) for _ in range(10)]
        
        for thread in threads:
            thread.start()
        
        for thread in threads:
            thread.join()
        
        # All instances should be the same object
        self.assertEqual(len(set(id(i) for i in instances)), 1)
    
    def test_initialized_flag(self):
        """Test that _initialized flag prevents re-initialization"""
        dm = DataManager()
        self.assertTrue(dm._initialized)
        
        # Create another reference
        dm2 = DataManager()
        self.assertTrue(dm2._initialized)


class TestDataManagerMQTTClientManagement(unittest.TestCase):
    """Test MQTT client management functionality"""
    
    def setUp(self):
        """Reset singleton and create fresh instance"""
        DataManager._instance = None
        self.dm = DataManager()
    
    def test_get_mqtt_client_initially_none(self):
        """Test that MQTT client is initially None"""
        self.assertIsNone(self.dm.get_mqtt_client())
    
    def test_set_mqtt_client(self):
        """Test setting MQTT client"""
        mock_client = Mock(spec=MQTTPowerClient)
        self.dm.set_mqtt_client(mock_client)
        
        retrieved_client = self.dm.get_mqtt_client()
        self.assertIs(retrieved_client, mock_client)
    
    def test_set_mqtt_client_thread_safety(self):
        """Test that setting MQTT client is thread-safe"""
        mock_clients = [Mock(spec=MQTTPowerClient) for _ in range(10)]
        
        def set_client(client):
            self.dm.set_mqtt_client(client)
            time.sleep(0.001)  # Small delay to increase chance of race conditions
        
        threads = [threading.Thread(target=set_client, args=(client,)) for client in mock_clients]
        
        for thread in threads:
            thread.start()
        
        for thread in threads:
            thread.join()
        
        # Should have one of the mock clients set
        final_client = self.dm.get_mqtt_client()
        self.assertIn(final_client, mock_clients)
    
    @patch('src.application.data_manager.MQTTPowerClient')
    def test_create_mqtt_client_success(self, mock_mqtt_class):
        """Test successful MQTT client creation"""
        mock_instance = Mock(spec=MQTTPowerClient)
        mock_mqtt_class.return_value = mock_instance
        
        client = self.dm.create_mqtt_client()
        
        self.assertIs(client, mock_instance)
        self.assertIs(self.dm.get_mqtt_client(), mock_instance)
        mock_mqtt_class.assert_called_once()
    
    @patch('src.application.data_manager.MQTTPowerClient')
    def test_create_mqtt_client_already_exists(self, mock_mqtt_class):
        """Test that create_mqtt_client returns existing client if already created"""
        mock_instance = Mock(spec=MQTTPowerClient)
        mock_mqtt_class.return_value = mock_instance
        
        client1 = self.dm.create_mqtt_client()
        client2 = self.dm.create_mqtt_client()
        
        self.assertIs(client1, client2)
        # Should only be called once
        mock_mqtt_class.assert_called_once()
    
    @patch('src.application.data_manager.MQTTPowerClient')
    @patch('builtins.print')
    def test_create_mqtt_client_error(self, mock_print, mock_mqtt_class):
        """Test MQTT client creation with exception"""
        mock_mqtt_class.side_effect = Exception("Connection failed")
        
        client = self.dm.create_mqtt_client()
        
        self.assertIsNone(client)
        self.assertIsNone(self.dm.get_mqtt_client())
        mock_print.assert_called()
        self.assertIn("MQTT client creation error", str(mock_print.call_args))


class TestDataManagerPowerDataManagement(unittest.TestCase):
    """Test power data management functionality"""
    
    def setUp(self):
        """Reset singleton and create fresh instance"""
        DataManager._instance = None
        self.dm = DataManager()
    
    def test_get_latest_power_data_initially_none(self):
        """Test that power data is initially None"""
        self.assertIsNone(self.dm.get_latest_power_data())
    
    @patch('builtins.print')
    def test_update_power_data(self, mock_print):
        """Test updating power data"""
        power = 1234.5678
        timestamp = datetime(2024, 1, 15, 12, 30, 0)
        
        self.dm.update_power_data(power, timestamp)
        
        data = self.dm.get_latest_power_data()
        self.assertIsNotNone(data)
        self.assertEqual(data['power'], 1234.57)  # Rounded to 2 decimals
        self.assertEqual(data['timestamp'], timestamp)
        
        # Verify print was called
        mock_print.assert_called()
        self.assertIn("1234.5678W", str(mock_print.call_args))
    
    def test_update_power_data_rounding(self):
        """Test that power values are rounded to 2 decimal places"""
        test_cases = [
            (1234.5678, 1234.57),
            (100.001, 100.0),
            (50.996, 51.0),  # Changed from 50.995 due to banker's rounding
            (0.0, 0.0),
        ]
        
        for input_power, expected_power in test_cases:
            timestamp = datetime.now()
            self.dm.update_power_data(input_power, timestamp)
            
            data = self.dm.get_latest_power_data()
            self.assertEqual(data['power'], expected_power)
    
    def test_get_latest_power_data_returns_copy(self):
        """Test that get_latest_power_data returns a copy, not the original"""
        power = 1000.0
        timestamp = datetime.now()
        self.dm.update_power_data(power, timestamp)
        
        data1 = self.dm.get_latest_power_data()
        data2 = self.dm.get_latest_power_data()
        
        # Should be equal but not the same object
        self.assertEqual(data1, data2)
        self.assertIsNot(data1, data2)
        
        # Modifying one should not affect the other
        data1['power'] = 9999
        self.assertEqual(data2['power'], 1000.0)
    
    def test_update_power_data_thread_safety(self):
        """Test that power data updates are thread-safe"""
        results = []
        
        def update_power(value):
            timestamp = datetime.now()
            self.dm.update_power_data(value, timestamp)
            time.sleep(0.001)
            data = self.dm.get_latest_power_data()
            if data:
                results.append(data['power'])
        
        threads = [threading.Thread(target=update_power, args=(i * 100,)) for i in range(10)]
        
        for thread in threads:
            thread.start()
        
        for thread in threads:
            thread.join()
        
        # Should have 10 results
        self.assertEqual(len(results), 10)
        
        # All results should be valid power values
        for result in results:
            self.assertIsInstance(result, (int, float))
            self.assertGreaterEqual(result, 0)
    
    def test_update_power_data_multiple_times(self):
        """Test that repeated updates replace previous data"""
        timestamps = [
            datetime(2024, 1, 15, 12, 0, 0),
            datetime(2024, 1, 15, 12, 30, 0),
            datetime(2024, 1, 15, 13, 0, 0),
        ]
        powers = [100.0, 200.0, 300.0]
        
        for power, timestamp in zip(powers, timestamps):
            self.dm.update_power_data(power, timestamp)
        
        # Should have only the last update
        data = self.dm.get_latest_power_data()
        self.assertEqual(data['power'], 300.0)
        self.assertEqual(data['timestamp'], timestamps[2])


class TestDataManagerClientConnectionManagement(unittest.TestCase):
    """Test client connection tracking functionality"""
    
    def setUp(self):
        """Reset singleton and create fresh instance"""
        DataManager._instance = None
        self.dm = DataManager()
    
    def test_initial_client_count(self):
        """Test that client count starts at zero"""
        self.assertEqual(self.dm.get_client_count(), 0)
        self.assertFalse(self.dm.has_connected_clients())
    
    @patch('builtins.print')
    def test_increment_clients(self, mock_print):
        """Test incrementing client count"""
        count = self.dm.increment_clients()
        
        self.assertEqual(count, 1)
        self.assertEqual(self.dm.get_client_count(), 1)
        self.assertTrue(self.dm.has_connected_clients())
        
        mock_print.assert_called()
        self.assertIn("Client connected", str(mock_print.call_args))
        self.assertIn("Total clients: 1", str(mock_print.call_args))
    
    @patch('builtins.print')
    def test_decrement_clients(self, mock_print):
        """Test decrementing client count"""
        self.dm.increment_clients()
        self.dm.increment_clients()
        
        count = self.dm.decrement_clients()
        
        self.assertEqual(count, 1)
        self.assertEqual(self.dm.get_client_count(), 1)
        self.assertTrue(self.dm.has_connected_clients())
        
        mock_print.assert_called()
        self.assertIn("Client disconnected", str(mock_print.call_args))
        self.assertIn("Total clients: 1", str(mock_print.call_args))
    
    def test_decrement_clients_below_zero(self):
        """Test that client count doesn't go below zero"""
        # Start with 0 clients
        self.assertEqual(self.dm.get_client_count(), 0)
        
        # Try to decrement
        count = self.dm.decrement_clients()
        
        self.assertEqual(count, 0)
        self.assertEqual(self.dm.get_client_count(), 0)
        self.assertFalse(self.dm.has_connected_clients())
    
    def test_multiple_client_connections(self):
        """Test multiple client connections and disconnections"""
        # Add 5 clients
        for i in range(5):
            count = self.dm.increment_clients()
            self.assertEqual(count, i + 1)
        
        self.assertEqual(self.dm.get_client_count(), 5)
        self.assertTrue(self.dm.has_connected_clients())
        
        # Remove 3 clients
        for i in range(3):
            count = self.dm.decrement_clients()
            self.assertEqual(count, 5 - i - 1)
        
        self.assertEqual(self.dm.get_client_count(), 2)
        self.assertTrue(self.dm.has_connected_clients())
        
        # Remove remaining clients
        self.dm.decrement_clients()
        self.dm.decrement_clients()
        
        self.assertEqual(self.dm.get_client_count(), 0)
        self.assertFalse(self.dm.has_connected_clients())
    
    def test_client_tracking_thread_safety(self):
        """Test that client tracking is thread-safe"""
        num_threads = 20
        
        def increment():
            for _ in range(10):
                self.dm.increment_clients()
                time.sleep(0.0001)
        
        def decrement():
            for _ in range(10):
                time.sleep(0.0001)
                self.dm.decrement_clients()
        
        # Create threads
        inc_threads = [threading.Thread(target=increment) for _ in range(num_threads // 2)]
        dec_threads = [threading.Thread(target=decrement) for _ in range(num_threads // 2)]
        
        all_threads = inc_threads + dec_threads
        
        # Start all threads
        for thread in all_threads:
            thread.start()
        
        # Wait for all threads
        for thread in all_threads:
            thread.join()
        
        # Final count should be 0 (10 increments * 10 threads = 100, 10 decrements * 10 threads = 100)
        final_count = self.dm.get_client_count()
        self.assertEqual(final_count, 0)
    
    def test_has_connected_clients(self):
        """Test has_connected_clients method"""
        # Initially no clients
        self.assertFalse(self.dm.has_connected_clients())
        
        # Add one client
        self.dm.increment_clients()
        self.assertTrue(self.dm.has_connected_clients())
        
        # Add more clients
        self.dm.increment_clients()
        self.dm.increment_clients()
        self.assertTrue(self.dm.has_connected_clients())
        
        # Remove all but one
        self.dm.decrement_clients()
        self.dm.decrement_clients()
        self.assertTrue(self.dm.has_connected_clients())
        
        # Remove last client
        self.dm.decrement_clients()
        self.assertFalse(self.dm.has_connected_clients())


class TestDataManagerIntegration(unittest.TestCase):
    """Integration tests for DataManager"""
    
    def setUp(self):
        """Reset singleton and create fresh instance"""
        DataManager._instance = None
        self.dm = DataManager()
    
    @patch('src.application.data_manager.MQTTPowerClient')
    @patch('builtins.print')
    def test_complete_workflow(self, mock_print, mock_mqtt_class):
        """Test a complete workflow with all features"""
        # Create MQTT client
        mock_instance = Mock(spec=MQTTPowerClient)
        mock_mqtt_class.return_value = mock_instance
        
        client = self.dm.create_mqtt_client()
        self.assertIsNotNone(client)
        
        # Simulate client connections
        self.dm.increment_clients()
        self.dm.increment_clients()
        self.assertEqual(self.dm.get_client_count(), 2)
        
        # Update power data
        timestamp = datetime(2024, 1, 15, 12, 30, 0)
        self.dm.update_power_data(1500.75, timestamp)
        
        data = self.dm.get_latest_power_data()
        self.assertEqual(data['power'], 1500.75)
        self.assertEqual(data['timestamp'], timestamp)
        
        # Disconnect one client
        self.dm.decrement_clients()
        self.assertEqual(self.dm.get_client_count(), 1)
        self.assertTrue(self.dm.has_connected_clients())
        
        # Update power again
        new_timestamp = datetime(2024, 1, 15, 12, 35, 0)
        self.dm.update_power_data(1600.25, new_timestamp)
        
        data = self.dm.get_latest_power_data()
        self.assertEqual(data['power'], 1600.25)
        self.assertEqual(data['timestamp'], new_timestamp)
        
        # Verify MQTT client is still the same
        self.assertIs(self.dm.get_mqtt_client(), client)
    
    def test_concurrent_operations(self):
        """Test concurrent access to all features"""
        results = {'power_updates': 0, 'client_changes': 0}
        
        def update_power():
            for i in range(5):
                timestamp = datetime.now()
                self.dm.update_power_data(i * 100, timestamp)
                results['power_updates'] += 1
                time.sleep(0.001)
        
        def manage_clients():
            for _ in range(5):
                self.dm.increment_clients()
                time.sleep(0.001)
                self.dm.decrement_clients()
                results['client_changes'] += 1
        
        threads = [
            threading.Thread(target=update_power),
            threading.Thread(target=update_power),
            threading.Thread(target=manage_clients),
            threading.Thread(target=manage_clients),
        ]
        
        for thread in threads:
            thread.start()
        
        for thread in threads:
            thread.join()
        
        # Verify operations completed
        self.assertEqual(results['power_updates'], 10)
        self.assertEqual(results['client_changes'], 10)
        
        # Verify data integrity
        data = self.dm.get_latest_power_data()
        self.assertIsNotNone(data)
        
        # Client count should be 0 (equal increments and decrements)
        self.assertEqual(self.dm.get_client_count(), 0)


if __name__ == '__main__':
    unittest.main()
