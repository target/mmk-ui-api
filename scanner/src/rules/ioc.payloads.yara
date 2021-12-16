rule ioc_payload_checkout_clear_cc {
  meta:
    description = "Checkout Clear CC"
    author = "Eric Brandel"
    reference = ""
    date = "2019-10-14"
  strings:
    $s1 = "4012000077777777"
  condition:
    all of them
}

rule ioc_payload_checkout_b64_cc {
  meta:
    description = "Checkout base64 CC"
    author = "Eric Brandel"
    reference = ""
    date = "2019-10-14"
  strings:
    $s1 = "NDAxMjAwMDA3Nzc3Nzc3Nw"
  condition:
    all of them
}
