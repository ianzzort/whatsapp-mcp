import os
import subprocess
import tempfile
from typing import List, Dict, Any, Optional
from mcp.server.fastmcp import FastMCP, Image
from whatsapp import (
    search_contacts as whatsapp_search_contacts,
    list_messages as whatsapp_list_messages,
    list_chats as whatsapp_list_chats,
    get_chat as whatsapp_get_chat,
    get_direct_chat_by_contact as whatsapp_get_direct_chat_by_contact,
    get_contact_chats as whatsapp_get_contact_chats,
    get_last_interaction as whatsapp_get_last_interaction,
    get_message_context as whatsapp_get_message_context,
    send_message as whatsapp_send_message,
    send_file as whatsapp_send_file,
    send_audio_message as whatsapp_audio_voice_message,
    download_media as whatsapp_download_media
)

# Initialize FastMCP server
mcp = FastMCP("whatsapp")

@mcp.tool()
def search_contacts(query: str) -> List[Dict[str, Any]]:
    """Search WhatsApp contacts by name or phone number.
    
    Args:
        query: Search term to match against contact names or phone numbers
    """
    contacts = whatsapp_search_contacts(query)
    return contacts

@mcp.tool()
def list_messages(
    after: Optional[str] = None,
    before: Optional[str] = None,
    sender_phone_number: Optional[str] = None,
    chat_jid: Optional[str] = None,
    query: Optional[str] = None,
    limit: int = 20,
    page: int = 0,
    include_context: bool = True,
    context_before: int = 1,
    context_after: int = 1
) -> List[Dict[str, Any]]:
    """Get WhatsApp messages matching specified criteria with optional context.
    
    Args:
        after: Optional ISO-8601 formatted string to only return messages after this date
        before: Optional ISO-8601 formatted string to only return messages before this date
        sender_phone_number: Optional phone number to filter messages by sender
        chat_jid: Optional chat JID to filter messages by chat
        query: Optional search term to filter messages by content
        limit: Maximum number of messages to return (default 20)
        page: Page number for pagination (default 0)
        include_context: Whether to include messages before and after matches (default True)
        context_before: Number of messages to include before each match (default 1)
        context_after: Number of messages to include after each match (default 1)
    """
    messages = whatsapp_list_messages(
        after=after,
        before=before,
        sender_phone_number=sender_phone_number,
        chat_jid=chat_jid,
        query=query,
        limit=limit,
        page=page,
        include_context=include_context,
        context_before=context_before,
        context_after=context_after
    )
    return messages

@mcp.tool()
def list_chats(
    query: Optional[str] = None,
    limit: int = 20,
    page: int = 0,
    include_last_message: bool = True,
    sort_by: str = "last_active"
) -> List[Dict[str, Any]]:
    """Get WhatsApp chats matching specified criteria.
    
    Args:
        query: Optional search term to filter chats by name or JID
        limit: Maximum number of chats to return (default 20)
        page: Page number for pagination (default 0)
        include_last_message: Whether to include the last message in each chat (default True)
        sort_by: Field to sort results by, either "last_active" or "name" (default "last_active")
    """
    chats = whatsapp_list_chats(
        query=query,
        limit=limit,
        page=page,
        include_last_message=include_last_message,
        sort_by=sort_by
    )
    return chats

@mcp.tool()
def get_chat(chat_jid: str, include_last_message: bool = True) -> Dict[str, Any]:
    """Get WhatsApp chat metadata by JID.
    
    Args:
        chat_jid: The JID of the chat to retrieve
        include_last_message: Whether to include the last message (default True)
    """
    chat = whatsapp_get_chat(chat_jid, include_last_message)
    return chat

@mcp.tool()
def get_direct_chat_by_contact(sender_phone_number: str) -> Dict[str, Any]:
    """Get WhatsApp chat metadata by sender phone number.
    
    Args:
        sender_phone_number: The phone number to search for
    """
    chat = whatsapp_get_direct_chat_by_contact(sender_phone_number)
    return chat

@mcp.tool()
def get_contact_chats(jid: str, limit: int = 20, page: int = 0) -> List[Dict[str, Any]]:
    """Get all WhatsApp chats involving the contact.
    
    Args:
        jid: The contact's JID to search for
        limit: Maximum number of chats to return (default 20)
        page: Page number for pagination (default 0)
    """
    chats = whatsapp_get_contact_chats(jid, limit, page)
    return chats

@mcp.tool()
def get_last_interaction(jid: str) -> str:
    """Get most recent WhatsApp message involving the contact.
    
    Args:
        jid: The JID of the contact to search for
    """
    message = whatsapp_get_last_interaction(jid)
    return message

@mcp.tool()
def get_message_context(
    message_id: str,
    before: int = 5,
    after: int = 5
) -> Dict[str, Any]:
    """Get context around a specific WhatsApp message.
    
    Args:
        message_id: The ID of the message to get context for
        before: Number of messages to include before the target message (default 5)
        after: Number of messages to include after the target message (default 5)
    """
    context = whatsapp_get_message_context(message_id, before, after)
    return context

@mcp.tool()
def send_message(
    recipient: str,
    message: str
) -> Dict[str, Any]:
    """Send a WhatsApp message to a person or group. For group chats use the JID.

    Args:
        recipient: The recipient - either a phone number with country code but no + or other symbols,
                 or a JID (e.g., "123456789@s.whatsapp.net" or a group JID like "123456789@g.us")
        message: The message text to send
    
    Returns:
        A dictionary containing success status and a status message
    """
    # Validate input
    if not recipient:
        return {
            "success": False,
            "message": "Recipient must be provided"
        }
    
    # Call the whatsapp_send_message function with the unified recipient parameter
    success, status_message = whatsapp_send_message(recipient, message)
    return {
        "success": success,
        "message": status_message
    }

@mcp.tool()
def send_file(recipient: str, media_path: str) -> Dict[str, Any]:
    """Send a file such as a picture, raw audio, video or document via WhatsApp to the specified recipient. For group messages use the JID.
    
    Args:
        recipient: The recipient - either a phone number with country code but no + or other symbols,
                 or a JID (e.g., "123456789@s.whatsapp.net" or a group JID like "123456789@g.us")
        media_path: The absolute path to the media file to send (image, video, document)
    
    Returns:
        A dictionary containing success status and a status message
    """
    
    # Call the whatsapp_send_file function
    success, status_message = whatsapp_send_file(recipient, media_path)
    return {
        "success": success,
        "message": status_message
    }

@mcp.tool()
def send_audio_message(recipient: str, media_path: str) -> Dict[str, Any]:
    """Send any audio file as a WhatsApp audio message to the specified recipient. For group messages use the JID. If it errors due to ffmpeg not being installed, use send_file instead.
    
    Args:
        recipient: The recipient - either a phone number with country code but no + or other symbols,
                 or a JID (e.g., "123456789@s.whatsapp.net" or a group JID like "123456789@g.us")
        media_path: The absolute path to the audio file to send (will be converted to Opus .ogg if it's not a .ogg file)
    
    Returns:
        A dictionary containing success status and a status message
    """
    success, status_message = whatsapp_audio_voice_message(recipient, media_path)
    return {
        "success": success,
        "message": status_message
    }

@mcp.tool()
def download_media(message_id: str, chat_jid: str) -> Dict[str, Any]:
    """Download media from a WhatsApp message and get the local file path.
    
    Args:
        message_id: The ID of the message containing the media
        chat_jid: The JID of the chat containing the message
    
    Returns:
        A dictionary containing success status, a status message, and the file path if successful
    """
    file_path = whatsapp_download_media(message_id, chat_jid)
    
    if file_path:
        return {
            "success": True,
            "message": "Media downloaded successfully",
            "file_path": file_path
        }
    else:
        return {
            "success": False,
            "message": "Failed to download media"
        }

_whisper_model = None

def _whisper():
    global _whisper_model
    if _whisper_model is None:
        from faster_whisper import WhisperModel
        _whisper_model = WhisperModel("small", device="cpu", compute_type="int8")
    return _whisper_model

def _transcribe_file(path: str, language: Optional[str] = None):
    segments, info = _whisper().transcribe(path, language=language)
    return info.language, " ".join(s.text.strip() for s in segments)

@mcp.tool()
def transcribe_audio(message_id: str, chat_jid: str, language: Optional[str] = None) -> Dict[str, Any]:
    """Download a WhatsApp audio/voice message and transcribe it to text. Use this to know what an [audio] message says.

    Args:
        message_id: The ID of the message containing the audio
        chat_jid: The JID of the chat containing the message
        language: Optional ISO language code (e.g. "es"); autodetected if omitted
    """
    path = whatsapp_download_media(message_id, chat_jid)
    if not path:
        return {"success": False, "message": "Failed to download audio"}
    lang, text = _transcribe_file(path, language)
    return {"success": True, "language": lang, "text": text}

@mcp.tool()
def view_image(message_id: str, chat_jid: str) -> Image:
    """Download an image from a WhatsApp message and return it so it can be seen and analyzed.

    Args:
        message_id: The ID of the message containing the image
        chat_jid: The JID of the chat containing the message
    """
    path = whatsapp_download_media(message_id, chat_jid)
    if not path:
        raise ValueError("Failed to download image")
    if os.path.getsize(path) > 900_000:
        small = tempfile.NamedTemporaryFile(suffix=".jpg", delete=False).name
        subprocess.run(
            ["ffmpeg", "-y", "-i", path, "-vf", "scale='min(1280,iw)':-2", small],
            capture_output=True, check=True
        )
        path = small
    return Image(path=path)

@mcp.tool()
def view_video(message_id: str, chat_jid: str, max_frames: int = 6):
    """Download a WhatsApp video, extract evenly spaced frames and transcribe its audio track so the video can be understood.

    Args:
        message_id: The ID of the message containing the video
        chat_jid: The JID of the chat containing the message
        max_frames: Maximum number of frames to extract (default 6)
    """
    path = whatsapp_download_media(message_id, chat_jid)
    if not path:
        return "Failed to download video"
    probe = subprocess.run(
        ["ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", path],
        capture_output=True, text=True
    )
    duration = float(probe.stdout.strip() or 0)
    fps = max_frames / duration if duration > 0 else 1
    out_dir = tempfile.mkdtemp()
    subprocess.run(
        ["ffmpeg", "-y", "-i", path, "-vf", f"fps={fps},scale='min(800,iw)':-2",
         "-frames:v", str(max_frames), os.path.join(out_dir, "frame%02d.jpg")],
        capture_output=True
    )
    result = [f"Video duration: {duration:.1f}s"]
    try:
        lang, text = _transcribe_file(path)
        if text.strip():
            result.append(f"Audio transcript ({lang}): {text}")
    except Exception:
        result.append("No audio transcript available")
    for frame in sorted(os.listdir(out_dir)):
        result.append(Image(path=os.path.join(out_dir, frame)))
    return result

@mcp.tool()
def read_document(message_id: str, chat_jid: str) -> Dict[str, Any]:
    """Download a WhatsApp document and extract its text. Supports PDF and plain-text files.

    Args:
        message_id: The ID of the message containing the document
        chat_jid: The JID of the chat containing the message
    """
    path = whatsapp_download_media(message_id, chat_jid)
    if not path:
        return {"success": False, "message": "Failed to download document"}
    ext = os.path.splitext(path)[1].lower()
    if ext == ".pdf":
        from pypdf import PdfReader
        text = "\n".join(page.extract_text() or "" for page in PdfReader(path).pages)
    elif ext in (".txt", ".csv", ".json", ".md", ".xml", ".html", ".log"):
        with open(path, encoding="utf-8", errors="replace") as f:
            text = f.read()
    else:
        return {"success": False, "message": f"Unsupported document type: {ext}", "file_path": path}
    return {"success": True, "text": text[:100_000], "file_path": path}

if __name__ == "__main__":
    # Initialize and run the server
    mcp.run(transport='stdio')
