"""
Microsoft Entra ID (Azure AD) Authentication Module

This module provides OAuth2 authentication using Microsoft Entra ID.
Users will be redirected to Microsoft login and must authenticate
with their organizational account.
"""

import os
from functools import wraps
from flask import Blueprint, redirect, url_for, session, request, current_app
import msal

auth_bp = Blueprint('auth', __name__, url_prefix='/auth')

# Configuration
AUTHORITY = None
CLIENT_ID = None
CLIENT_SECRET = None
REDIRECT_PATH = "/auth/callback"
SCOPE = ["User.Read"]


def init_auth(app):
    """Initialize authentication configuration."""
    global AUTHORITY, CLIENT_ID, CLIENT_SECRET
    
    tenant_id = os.getenv('AZURE_TENANT_ID')
    client_id_val = os.getenv('AZURE_CLIENT_ID')
    client_secret_val = os.getenv('AZURE_CLIENT_SECRET')
    
    # Check for placeholders or missing values
    # Common placeholders to ignore
    placeholders = ["your_", "placeholder", "example_of", "value_here"]
    
    is_valid_config = True
    if not all([tenant_id, client_id_val, client_secret_val]):
        is_valid_config = False
    elif any(p in (tenant_id or "").lower() for p in placeholders):
        is_valid_config = False
    elif any(p in (client_id_val or "").lower() for p in placeholders):
        is_valid_config = False
        
    if not is_valid_config:
        app.logger.warning("Azure AD authentication not configured (or placeholders detected). Auth will be bypassed.")
        # Ensure globals are None so is_authenticated() returns True (bypass)
        CLIENT_ID = None
        CLIENT_SECRET = None
        AUTHORITY = None
        return False
        
    CLIENT_ID = client_id_val
    CLIENT_SECRET = client_secret_val
    AUTHORITY = f"https://login.microsoftonline.com/{tenant_id}"
    
    return True


def _build_msal_app(cache=None):
    """Build MSAL confidential client application."""
    return msal.ConfidentialClientApplication(
        CLIENT_ID,
        authority=AUTHORITY,
        client_credential=CLIENT_SECRET,
        token_cache=cache
    )


def _build_auth_url(redirect_uri):
    """Build the authorization URL for login."""
    return _build_msal_app().get_authorization_request_url(
        SCOPE,
        redirect_uri=redirect_uri
    )


def _get_token_from_cache():
    """Get token from cache if available."""
    cache = msal.SerializableTokenCache()
    if session.get("token_cache"):
        cache.deserialize(session["token_cache"])
    
    app = _build_msal_app(cache)
    accounts = app.get_accounts()
    
    if accounts:
        result = app.acquire_token_silent(SCOPE, account=accounts[0])
        if cache.has_state_changed:
            session["token_cache"] = cache.serialize()
        return result
    return None


def is_authenticated():
    """Check if user is authenticated."""
    # If auth is not configured, allow access
    if not CLIENT_ID:
        return True
    return session.get("user") is not None


def login_required(f):
    """Decorator to require authentication for a route."""
    @wraps(f)
    def decorated_function(*args, **kwargs):
        if not is_authenticated():
            # Store the original URL to redirect back after login
            session["next_url"] = request.url
            return redirect(url_for("auth.login"))
        return f(*args, **kwargs)
    return decorated_function


def get_current_user():
    """Get the current logged-in user info."""
    return session.get("user")


@auth_bp.route("/login")
def login():
    """Initiate login flow."""
    if not CLIENT_ID:
        return redirect(url_for("index"))
    
    redirect_uri = request.url_root.rstrip('/') + REDIRECT_PATH
    auth_url = _build_auth_url(redirect_uri)
    return redirect(auth_url)


@auth_bp.route("/callback")
def callback():
    """Handle OAuth callback from Microsoft."""
    if not CLIENT_ID:
        return redirect(url_for("index"))
    
    if "error" in request.args:
        return f"Login error: {request.args.get('error_description', 'Unknown error')}", 400
    
    if "code" not in request.args:
        return redirect(url_for("auth.login"))
    
    cache = msal.SerializableTokenCache()
    app = _build_msal_app(cache)
    
    redirect_uri = request.url_root.rstrip('/') + REDIRECT_PATH
    result = app.acquire_token_by_authorization_code(
        request.args["code"],
        scopes=SCOPE,
        redirect_uri=redirect_uri
    )
    
    if "error" in result:
        return f"Token error: {result.get('error_description', 'Unknown error')}", 400
    
    # Store user info in session
    session["user"] = result.get("id_token_claims")
    session["token_cache"] = cache.serialize()
    
    # Redirect to original URL or home
    next_url = session.pop("next_url", None)
    return redirect(next_url or url_for("index"))


@auth_bp.route("/logout")
def logout():
    """Log out the user."""
    session.clear()
    
    if not CLIENT_ID:
        return redirect(url_for("index"))
    
    # Redirect to Microsoft logout
    logout_url = f"{AUTHORITY}/oauth2/v2.0/logout"
    post_logout_redirect = request.url_root.rstrip('/')
    return redirect(f"{logout_url}?post_logout_redirect_uri={post_logout_redirect}")


@auth_bp.route("/user")
def user_info():
    """Return current user info as JSON."""
    user = get_current_user()
    if user:
        return {
            "name": user.get("name"),
            "email": user.get("preferred_username"),
            "authenticated": True
        }
    return {"authenticated": False}
