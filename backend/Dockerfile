FROM python:3.11-slim

# Create non-root user
RUN useradd -m -u 1000 rpki && \
    mkdir -p /app/state && \
    chown -R rpki:rpki /app

WORKDIR /app

# Install dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy application code
COPY --chown=rpki:rpki . .

# Switch to non-root user
USER rpki

# Expose API port
EXPOSE 8080

# Run the backend
CMD ["python", "-u", "main.py"]