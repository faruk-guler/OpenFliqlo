GoSaatVeTarih.scr - GPO ile Dağıtım Rehberi
=============================================

Bu doküman, hazırlanan ekran koruyucunun Active Directory (GPO) üzerinden bilgisayarlara nasıl uygulanacağını açıklar.

ADIM 1: Dosyayı Bilgisayarlara Dağıtma
--------------------------------------
Ekran koruyucunun çalışması için dosyanın yerel bilgisayarda (C:\Windows\System32) olması en sağlıklı yöntemdir.

1. GPMC açın ve yeni bir GPO oluşturun.
2. Computer Configuration -> Preferences -> Windows Settings -> Files yoluna gidin.
3. Sağ tık -> New -> File seçin.
   - Source File: \\ServerName\Share\GoSaatVeTarih.scr
   - Destination File: C:\Windows\System32\GoSaatVeTarih.scr
4. "Action" kısmını "Update" olarak ayarlayın.

ADIM 2: Ekran Koruyucuyu Aktif Etme
-----------------------------------
1. User Configuration -> Policies -> Administrative Templates -> Control Panel -> Personalization yoluna gidin.
2. Şu ayarları yapılandırın:
   - "Enable screen saver": Enabled
   - "Force specific screen saver": Enabled -> C:\Windows\System32\GoSaatVeTarih.scr
   - "Screen saver timeout": Enabled -> 300 (5 Dakika)

ADIM 3: Uygulama
----------------
GPO'yu ilgili OU üzerine bağlayın.
