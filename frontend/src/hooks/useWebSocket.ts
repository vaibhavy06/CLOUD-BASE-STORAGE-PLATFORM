'use client';

import { useEffect, useRef } from 'react';
import { useAuthStore } from '../store/authStore';
import { useNotificationStore } from '../store/notificationStore';

export function useWebSocket() {
  const accessToken = useAuthStore((state) => state.accessToken);
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);
  const addNotification = useNotificationStore((state) => state.addNotification);
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    if (!isAuthenticated || !accessToken) {
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
      return;
    }

    let reconnectTimeout: NodeJS.Timeout;
    let reconnectDelay = 2000; // start with 2s

    const connect = () => {
      const apiHost = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8081';
      const wsHost = apiHost.replace(/^http/, 'ws');
      const wsUrl = `${wsHost}/api/ws?token=${accessToken}`;
      console.log('Connecting to WebSocket event stream...');
      const ws = new WebSocket(wsUrl);
      wsRef.current = ws;

      ws.onopen = () => {
        console.log('WebSocket connection established.');
        reconnectDelay = 2000; // reset reconnect delay
      };

      ws.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          console.log('WebSocket event received:', data);

          if (data.type === 'notification') {
            addNotification(data.title, data.message, data.notification_type || 'info');
          }
        } catch (err) {
          console.error('Failed to parse WebSocket message data', err);
        }
      };

      ws.onclose = (e) => {
        console.log(`WebSocket stream closed: ${e.reason}. Attempting reconnect...`);
        wsRef.current = null;
        
        // Don't reconnect if user logged out
        if (useAuthStore.getState().isAuthenticated) {
          reconnectTimeout = setTimeout(() => {
            reconnectDelay = Math.min(reconnectDelay * 2, 30000); // exponential backoff capped at 30s
            connect();
          }, reconnectDelay);
        }
      };

      ws.onerror = (err) => {
        console.error('WebSocket connection error:', err);
        ws.close();
      };
    };

    connect();

    return () => {
      clearTimeout(reconnectTimeout);
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
    };
  }, [isAuthenticated, accessToken, addNotification]);
}
