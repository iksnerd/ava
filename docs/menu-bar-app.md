# The menu bar app

Ava in the menu bar, for someone who would rather not use Terminal. A **Set up Ava** button in it
installs what it needs. The one thing to install by hand first is Homebrew, which means copying one
line from a website into Terminal; step 2 walks through it.

← [Back to the README](../README.md)

## 1. Check the Mac

Apple menu (top left) → **About This Mac**. **Chip** has to be an Apple M-series chip (M1 or
later) and **macOS** 13 or later. On an Intel Mac, the app cannot speak with the natural voice.

## 2. Install Homebrew

Skip this if Homebrew is already installed. The app checks, and links you back here if it isn't.

1. Open **Terminal** (press ⌘-Space, type `Terminal`, press Return).
2. Go to [brew.sh](https://brew.sh), copy the line under "Install Homebrew", paste it into
   Terminal and press Return.
3. When it asks for your password, type your Mac login password and press Return. Nothing appears
   as you type.
4. It may install the Command Line Tools first. The whole thing can take 10 minutes or more. Wait
   until it prints **Installation successful!**
5. It ends with "Next steps" and a few lines starting with `echo`. Ava does not need them. You can
   close Terminal now.

## 3. Install Ava

1. Download [`Ava.dmg`](https://github.com/iksnerd/ava/releases/latest/download/Ava.dmg) and open
   it from your Downloads folder.
2. A window opens showing the Ava icon.
   - If there is an **Applications** folder next to Ava, drag Ava onto it.
   - If Ava is alone in the window, click **Finder** in the Dock to open a second window, find
     **Applications** in the list on its left, and drag Ava from the first window onto it.
3. Open **Applications** and double-click **Ava**. macOS says it "could not verify Ava is free of
   malware", because the app is not signed. Click **Done**.
4. Open System Settings → **Privacy & Security** and scroll down to Security. Next to the message
   about Ava, click **Open Anyway**. In the box that pops up, click **Open Anyway** again and enter
   your password or use Touch ID. Ava opens by itself; if it does not, double-click it in
   Applications again. You only do this once.

## 4. Set it up

1. A waveform icon appears in the menu bar at the top right. Ava has no window of its own;
   everything happens from that icon. If you cannot see it, the menu bar may be too full: on a
   MacBook with a notch, icons that do not fit are hidden behind it. Quit a menu bar app you do not
   need, or hold ⌘ and drag other icons out of the way.
2. Click the waveform. A **Setup** card at the top says what is missing. Click **Set up Ava**. It
   downloads about 1.7 GB and takes a few minutes; you can close the panel meanwhile.
3. If the card says Homebrew is missing, go back to step 2, then click **Try again**. If setup
   stopped for another reason, click **Try again** once. If it stops again, copy the message it
   shows and post it as an [issue](https://github.com/iksnerd/ava/issues) or send it to whoever
   pointed you to Ava.

Check: the Setup card goes away. Click **Speak a Test Phrase** at the bottom of the panel, and a
natural voice speaks. The first time takes a few seconds while the voice engine starts.

## 5. Dictate

1. Click in the app you want to type into, where the text should go.
2. Click the waveform, then **Dictate**. The panel closes and Ava starts listening; nothing shows
   on screen.
3. Say a sentence, then stay quiet for 2 seconds. A short pop sound means it has finished, and the
   text is pasted where you clicked.

- The first time, macOS asks whether **Ava** may use the **Microphone**. Click **Allow**.
- Pasting needs **Accessibility**. If macOS says Ava wants to control your computer, open System
  Settings → Privacy & Security → **Accessibility**, turn **Ava** on, and dictate again.

Check: the sentence appears where your cursor was.

To change the voice, speed or volume, see the [app's own README](../AvaMenuBar/README.md).
