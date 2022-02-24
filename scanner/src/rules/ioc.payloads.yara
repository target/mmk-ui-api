rule ioc_payload_checkout_clear_cc {
  meta:
    description = "Checkout Clear CC"
    author = "Eric Brandel"
    reference = ""
    date = "2019-10-14"
  strings:
    $cc_num = "4111123412341234"
    $email = "foo.bar@example.com" nocase
    $firstname = "Kevin" nocase
    $lastname = "Flynn" nocase
  condition:
    any of them
}

rule ioc_payload_checkout_b64_cc {
  meta:
    description = "Checkout base64 CC"
    author = "Eric Brandel"
    reference = ""
    date = "2019-10-14"
  strings:
    $cc_num = "4111123412341234" base64
    $email = "foo.bar@example.com" base64
    $firstname = "Kevin" base64
    $lastname = "Flynn" base64
  condition:
    any of them
}

rule fetch_abnormal_content {
  meta:
    description = "Detects a fetch to abnormal content: fonts, images, and css"
    author = "Eric Brandel"
  strings:
    $resource = "resourceType\":\"fetch\""
    $contentFont = "content-type\":\"font"
    $contentImage = "content-type\":\"image"
    $contentCSS = "content-type\":\"text/css"
  condition:
    $resource and any of ($content*) 
}

rule fetch_exfil_image {
  meta:
    description = "Detects a fetch that is used for exfil via image post"
    author = "Eric Brandel"
  strings:
    $resource = "resourceType\":\"fetch\""
    $method = "method\":\"POST\""
    $contentImageRequest = "content-type\":\"multipart/form-data; boundary"
    $contentImage = "content-type\":\"image/x-icon"
  condition:
    $resource and $method and all of ($content*) 
}

rule xhr_abnormal_endpoint {
  meta:
    description = "Detects an XHR POST request to abnromal endpoints: css and ico"
    author = "Eric Brandel"
  strings:
    $resource = "resourceType\":\"xhr\""
    $contentType = "content-type\":\"text/html"
    $status = "\"status\":200"
    $urlRegex = /\"url\":\"http(s)?:\/\/([0-9a-z\.\/])*\.(css|ico)/
  condition:
    all of them 
}

