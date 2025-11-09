#!/usr/bin/env python3

import requests
import sys
import io
from PIL import Image

def test_media_upload(base_url, token):
    """Test media upload and verify CDN URLs are properly generated"""
    
    print("Testing media upload and CDN URL generation...")
    
    # Create a simple test image
    img = Image.new('RGB', (100, 100), color='red')
    img_bytes = io.BytesIO()
    img.save(img_bytes, format='PNG')
    img_bytes.seek(0)
    
    # Upload the image
    files = {
        'file': ('test.png', img_bytes, 'image/png'),
        'description': (None, 'Test image for CDN verification')
    }
    
    headers = {
        'Authorization': f'Bearer {token}'
    }
    
    print(f"Uploading image to {base_url}/api/v1/media...")
    response = requests.post(f"{base_url}/api/v1/media", files=files, headers=headers)
    
    if response.status_code != 200:
        print(f"❌ Upload failed: {response.status_code}")
        print(response.text)
        return False
    
    media = response.json()
    print(f"✅ Media uploaded successfully!")
    print(f"   ID: {media['id']}")
    print(f"   URL: {media['url']}")
    
    # Check if URL uses CDN domain
    if base_url.replace('https://', '') in media['url']:
        print("❌ Media URL is using main domain instead of CDN domain")
        return False
    
    if 'media.' in media['url']:
        print("✅ Media URL correctly uses CDN subdomain")
    elif 's3.amazonaws.com' in media['url']:
        print("⚠️  Media URL is using direct S3 URL (CDN may not be configured)")
    
    # Try to fetch the image via the CDN URL
    print(f"\nFetching image from CDN URL...")
    img_response = requests.get(media['url'])
    
    if img_response.status_code == 200:
        print("✅ Successfully fetched image from CDN")
        print(f"   Content-Type: {img_response.headers.get('Content-Type')}")
        print(f"   Content-Length: {img_response.headers.get('Content-Length')} bytes")
        
        # Check cache headers
        cache_control = img_response.headers.get('Cache-Control')
        if cache_control:
            print(f"   Cache-Control: {cache_control}")
            if 'max-age=31536000' in cache_control:
                print("   ✅ Long-term caching enabled")
        
        # Check if served by CloudFront
        cf_headers = {k: v for k, v in img_response.headers.items() if 'cloudfront' in k.lower()}
        if cf_headers:
            print("   ✅ Served by CloudFront CDN")
            for k, v in cf_headers.items():
                print(f"   {k}: {v}")
    else:
        print(f"❌ Failed to fetch image: {img_response.status_code}")
        return False
    
    # Test updating media metadata
    print(f"\nTesting media update...")
    update_data = {
        'description': 'Updated test image description',
        'focus': '0.5,-0.5'
    }
    
    update_response = requests.put(
        f"{base_url}/api/v1/media/{media['id']}", 
        json=update_data, 
        headers=headers
    )
    
    if update_response.status_code == 200:
        updated_media = update_response.json()
        print("✅ Media metadata updated successfully")
        print(f"   Description: {updated_media['description']}")
        if 'focus' in updated_media.get('meta', {}):
            print(f"   Focus: {updated_media['meta']['focus']}")
    else:
        print(f"❌ Failed to update media: {update_response.status_code}")
    
    return True

if __name__ == "__main__":
    if len(sys.argv) < 3:
        print("Usage: python test_media_urls.py <base_url> <access_token>")
        print("Example: python test_media_urls.py https://example.com YOUR_TOKEN")
        sys.exit(1)
    
    base_url = sys.argv[1].rstrip('/')
    token = sys.argv[2]
    
    try:
        success = test_media_upload(base_url, token)
        if success:
            print("\n✅ All media CDN tests passed!")
        else:
            print("\n❌ Some tests failed")
            sys.exit(1)
    except Exception as e:
        print(f"\n❌ Test error: {e}")
        sys.exit(1) 
