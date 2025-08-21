#!/bin/bash

# Start ollama server in the background
/bin/ollama serve &
OLLAMA_PID=$!

# Wait for ollama to be ready
echo "Waiting for Ollama server to start..."
sleep 5
echo "Ollama server is ready!"

# Pull the model if OLLAMA_MODEL is set
if [ -n "$OLLAMA_MODEL" ]; then
    echo "Pulling model: $OLLAMA_MODEL"
    /bin/ollama pull "$OLLAMA_MODEL"
    echo "Model $OLLAMA_MODEL is ready!"
else
    echo "No OLLAMA_MODEL specified, skipping model download"
fi

# Wait for the ollama server process
wait $OLLAMA_PID
