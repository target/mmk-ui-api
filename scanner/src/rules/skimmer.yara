rule digital_skimmer_coffemokko {
   meta:
    description = "CoffeMokko Digital Skimmer"
    author = "Eric Brandel"
    reference = "https://www.group-ib.com/blog/coffemokko"
    date = "2019-10-14"
   strings:
    $re1 = /var _\$_[0-9a-fA-F]{4}=\(function\(.,.\)/
    $re2 = /(\.join\(.\)\.split\(.\)){4}/
    $s1 = "encode:function("
    $s2 = "String.fromCharCode(127)"
    $s3 = "setTimeout"
    $s4 = "_keyStr"
   condition:
    all of them
}

rule digital_skimmer_inter {
   meta:
    description = "Inter Digital Skimmer"
    author = "Eric Brandel"
    reference = ""
    date = "2019-10-16"
   strings:
    $s1 = "SaveParam"
    $s2 = "SaveAllFields"
    $s3 = "SendData"
    $s4 = "TrySend"
    $s5 = "GetCCInfo"
    $s6 = "LoadImage"
    $s7 = "GetImageUrl"
    $s8 = "GetFromStorage"
    // uniqueish strings
    $s9 = "Cookies.set(\"$s\""
    $s10 = "Cookies.get(\"$s\")"
    $s11 = "Cookies.get(\"$sent\")"
   condition:
    6 of them
}

rule digital_skimmer_slowaes {
  meta:
    description = "Slow AES Encryption"
    author = "Eric Brandel"
    reference = "https://code.google.com/archive/p/slowaes/"
    date = "2019-10-14"
  strings:
    $s1 = "slowaes" nocase
    $s2 = "generatekey" nocase
  condition:
    all of them
}

rule digital_skimmer_gibberish {
  meta:
    description = "GibberishAES"
    author = "Eric Brandel"
    reference = "https://github.com/mdp/gibberish-aes"
    date = "2019-10-14"
  strings:
    $s1 = "gibberishaes" nocase
    $s2 = "maybe bad key" nocase
  condition:
    all of them
}

rule digital_skimmer_obfuscated_gibberish {
  meta:
    description = "Obfuscated GibberishAES"
    author = "Eric Brandel"
    reference = "https://github.com/mdp/gibberish-aes"
    date = "2019-10-14"
  strings:
    $s1 = "encryptblock" nocase
    $s2 = "decryptblock" nocase
    $s3 = "rawencrypt" nocase
    $s4 = "rawdecrypt" nocase
  condition:
    all of them
}

rule digital_skimmer_cryptojs {
  meta:
    description = "CryptoJS"
    author = "Eric Brandel"
    reference = "https://github.com/brix/crypto-js"
    date = "2019-10-14"
  strings:
    $s1 = "cryptojs" nocase
    $s2 = "malformed utf-8 data" nocase
    $s3 = "wordarray" nocase
  condition:
    all of them
}

rule digital_skimmer_unknown_encrypt {
  meta:
    description = "Unknown Encryption"
    author = "Eric Brandel"
    reference = ""
    date = "2019-10-14"
  strings:
    $s1 = "encryptdata"
    $hex_string = { 65 6E 63 6F 64 65 }
    $s3 = "genkey,geniv,encrypt"
  condition:
    ($s1 and $hex_string) or $s3
}

rule digital_skimmer_jsencrypt {
  meta:
    description = "JSEncrypt"
    author = "Eric Brandel"
    reference = "https://github.com/travist/jsencrypt"
    date = "2019-10-14"
  strings:
    $s1 = "jsencrypt" nocase
    $s2 = "hex encoding incomplete" nocase
  condition:
    all of them
}

rule digital_skimmer_base_encrypt {
  meta:
    description = "Base Encryption"
    author = "Eric Brandel"
    reference = ""
    date = "2019-10-14"
  strings:
    $s1 = "addroundkey"
    $s2 = "galois_multiplication"
    $s3 = "modeofoperation"
  condition:
    all of them
}

rule digital_skimmer_caesar_obf {
  meta:
    description = "Caesar Obfuscation"
    author = "Eric Brandel"
    reference = ""
    date = "2019-10-14"
  strings:
    $s1 = "xx = \"\".constructor"
    $s2 = "xx=\"\".constructor"
  condition:
    any of them
}

rule digital_skimmer_freshchat_obf {
  meta:
    description = "freshchat Obfuscation"
    author = "Eric Brandel"
    reference = ""
    date = "2019-10-14"
  strings:
    $re1 = /\\x[a-fA-F0-9]{2}/
    $re2 = /\\u00[a-fA-F0-9]{2}/
    // Unable to get yara to match on this
    // $re3 = /[a-zA-Z0-9]{4}.[a-zA-Z0-9]{3}=function/
    $s1 = "jQuery(document["
    $s2 = "return typeof"
    $s3 = "decodeURI"
    $s4 = ">>>"
    $s5 = "<<"
    // Unable to get yara to match this
    // $s6 = "==='function'"
  condition:
    all of them
}

rule digital_skimmer_giveme_obf {
  meta:
    description = "givemejs Obfuscation"
    author = "Eric Brandel"
    reference = ""
    date = "2019-10-14"
  strings:
    $re1 = /0w/
    $re2 = /\(function [a-zA-Z0-9]{3}\(\)/
    $s1 = "String.fromCharCode"
  condition:
    (#re1 > 3 and $s1 and $re2)
}

rule digital_skimmer_jquery_mask {
  meta:
    description = "jquery.mask"
    author = "Eric Brandel"
    reference = ""
    date = "2019-10-14"
  strings:
    $s1 = "jquery.mask.js"
    $s2 = "var Mask = function"
    $s3 = "Escobar"
  condition:
    all of them
}

rule digital_skimmer_obfuscatorio_obf {
  meta:
    description = "obfuscatorio Obfuscation"
    author = "Eric Brandel"
    reference = ""
    date = "2019-10-14"
  strings:
    $re1 = /var _0x[a-fA-F0-9]{4} *\= *\[/
    $re2 = /(\\x[a-fA-F0-9]{2}){4,}/
  condition:
    $re1 and #re2 > 4
}

rule digital_skimmer_sniffa_loader {
  meta:
    description = "The skimmer loader for mr.Sniffa"
  strings:
    $onload = "window.onload=function(){userID"
    $url = "//static.xx.fbcdn.net.com"
    $settime = /setTimeout\([A-Za-z]{4}\(\),1500\)/
  condition:
    all of them
}

rule digital_skimmer_whitespace
{
  meta:
    description = "Looks for an abundance of whitespace in a file"
    author = "Eric Brandel"
    reference = "mr.Sniffa"
    date = "2022-01-18"
  strings:
    $ttt = /\s\s\s\s\s\s\s\s\s\s\s\s/
  condition:
    #ttt > 100
}
