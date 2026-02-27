import os
import sys
from flask import Flask, render_template, request, redirect, url_for, flash, session
from flask_session import Session
from dotenv import load_dotenv

# Add parent directory to path for imports
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from services.report_service import ReportService
from auth import auth_bp, init_auth, login_required, get_current_user

# Load environment variables
load_dotenv()

# Initialize Flask app
app = Flask(__name__)

# Configure session for authentication
secret_key = os.getenv("FLASK_SECRET_KEY")
if not secret_key:
    app.logger.warning("FLASK_SECRET_KEY not set! Sessions will reset on restart. Set this in production.")
    secret_key = os.urandom(24).hex()

app.config["SECRET_KEY"] = secret_key
app.config["SESSION_TYPE"] = "filesystem"
app.config["SESSION_FILE_DIR"] = os.path.join(os.path.dirname(os.path.abspath(__file__)), "flask_session")
app.config["SESSION_PERMANENT"] = False
Session(app)

# Initialize and register authentication
init_auth(app)
app.register_blueprint(auth_bp)

# Initialize report service
report_service = ReportService()


@app.route("/", methods=["GET"])
@login_required
def index():
    """Render the main web interface with member selection."""
    user = get_current_user()
    try:
        members = report_service.get_members()
        return render_template("index.html", members=members, report=None, user=user)
    except Exception as e:
        return render_template("index.html", members=[], error=str(e), report=None, user=user)


@app.route("/generate", methods=["POST"])
@login_required
def generate():
    """Generate report based on form submission."""
    user = get_current_user()
    try:
        # Get form data
        member_id = request.form.get("member_id")
        days = int(request.form.get("days", 7))
        compare_projects = request.form.get("compare_projects") == "on"
        send_email = request.form.get("send_email") == "on"
        
        if not member_id:
            members = report_service.get_members()
            return render_template(
                "index.html",
                members=members,
                error="Please select a team member",
                report=None
            )
        
        # Generate the report
        result = report_service.generate_report(
            member_id=member_id,
            days=days,
            compare_projects=compare_projects,
            send_email=send_email
        )
        
        # Get members for the dropdown
        members = report_service.get_members()
        
        if result["success"]:
            # Build full report text
            report_text = result["report_body"]
            if result.get("comparison_summary"):
                report_text += "\n\n" + "=" * 50 + "\n"
                report_text += "PROJECT COMPARISON\n"
                report_text += "=" * 50 + "\n"
                report_text += result["comparison_summary"]
            
            success_message = "Report generated successfully!"
            if result.get("email_sent"):
                success_message += " Email has been sent."
            
            return render_template(
                "index.html",
                members=members,
                report={
                    "text": report_text,
                    "subject": result["report_subject"],
                    "member_id": member_id,
                    "ai_provider": result.get("ai_provider", "AI"),
                    "entry_count": result.get("entry_count", 0),
                    "project_count": result.get("project_count", 0)
                },
                success=success_message,
                selected_member=member_id,
                compare_projects=compare_projects,
                send_email=send_email
            )
        else:
            return render_template(
                "index.html",
                members=members,
                error=result.get("error", "Unknown error occurred"),
                report=None,
                selected_member=member_id,
                compare_projects=compare_projects,
                send_email=send_email
            )
            
    except Exception as e:
        members = report_service.get_members()
        return render_template(
            "index.html",
            members=members,
            error=f"Error generating report: {str(e)}",
            report=None
        )


@app.route("/download", methods=["POST"])
def download():
    """Download the report as a text file."""
    from flask import make_response
    from datetime import datetime
    
    report_text = request.form.get("report_text", "")
    report_subject = request.form.get("report_subject", "Weekly Report")
    
    # Create response
    response = make_response(report_text)
    response.headers["Content-Type"] = "text/plain"
    response.headers["Content-Disposition"] = f"attachment; filename=weekly_report_{datetime.now().strftime('%Y-%m-%d')}.txt"
    
    return response


@app.route("/health", methods=["GET"])
def health():
    """Health check endpoint for monitoring."""
    return {"status": "healthy", "service": "weekly-report-generator"}


if __name__ == "__main__":
    # Development server
    port = int(os.getenv("WEB_PORT", 5000))
    host = os.getenv("WEB_HOST", "0.0.0.0")
    debug = os.getenv("FLASK_DEBUG", "False").lower() == "true"
    
    print("=" * 50)
    print("Weekly Project Report Generator - Web Interface")
    print("=" * 50)
    
    if debug:
        print(f"⚠️  Running in DEVELOPMENT mode")
        print(f"   Starting server at http://{host}:{port}")
    else:
        print(f"✅ Running in PRODUCTION mode (Flask internal server)")
        print(f"   For true production, use: gunicorn --bind {host}:{port} app:app")
        print(f"   Listening at http://{host}:{port}")
        
    print("Press CTRL+C to quit")
    print()
    
    app.run(host=host, port=port, debug=debug)
