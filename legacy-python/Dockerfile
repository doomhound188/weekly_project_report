FROM python:3.11-slim

# Set timezone to Eastern for proper Friday scheduling
ENV TZ=America/Toronto
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

# Install cron
RUN apt-get update && apt-get install -y cron && rm -rf /var/lib/apt/lists/*

# Set working directory
WORKDIR /app

# Copy requirements and install dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy application files
COPY *.py ./
COPY services/ ./services/
COPY static/ ./static/
COPY templates/ ./templates/

# Copy entrypoint and crontab
COPY entrypoint.sh /entrypoint.sh
COPY crontab /etc/cron.d/weekly-report

# Set permissions
RUN chmod +x /entrypoint.sh
RUN chmod 0644 /etc/cron.d/weekly-report
RUN crontab /etc/cron.d/weekly-report

# Create log file
RUN touch /var/log/cron.log

# Expose port for web interface
EXPOSE 5000

# Set entrypoint
ENTRYPOINT ["/entrypoint.sh"]
