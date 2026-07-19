stretch goals:

    3. A idea is paid service for apikeys already set up for user, or the 2nd optino being user configure their own api keys; however I current have the github repo public. However, for the paid one, do we have to minimize privacy?

    - the main issue is the difficulty in obtaining the api keys, it will turn most people off


    ```     
        Context Summary: Mogi App API Architecture
        App Description: Mogi, an AI interviewer application.
        Core Challenge: Balancing user setup friction (managing API keys) against the developer's financial risk of covering API compute costs.
        Resolution: A dual-tier "Freemium" model that separates casual users from power users, while neutralizing the high cost of premium voice generation.

        Tier 1: Bring Your Own Key (BYOK)
        Target: Power users and developers.

        Cost to User: App is free to use; user pays their own API costs directly to the providers.

        User Experience: High friction setup, but absolute freedom.

        Capabilities: Users input their own OpenRouter, Google, and ElevenLabs keys. This allows them to manually select any LLM model they want via OpenRouter.

        Developer Risk: $0.

        Tier 2: Paid Subscription (e.g., $5/Month)
        Target: Casual users who want immediate access.

        Cost to User: Flat monthly fee.

        User Experience: Zero setup friction. The app works instantly out of the box.

        Capabilities: Uses preconfigured, cost-efficient models selected by the developer via OpenRouter and Google STT.

        Developer Risk: Managed. Must implement a backend proxy to hide master API keys and enforce strict internal usage caps (e.g., an internal credit system or time limits) so users don't burn through more than $5 of compute.

        The Premium Voice Strategy (Cost Control)
        The Problem: ElevenLabs (Text-to-Speech) is too expensive to bundle into a cheap subscription without severe financial risk.

        The Solution: Make ElevenLabs strictly optional across both tiers.

        Default: The app relies on free, device-native text-to-speech for all users.

        Upgrade: If any user (Free or Paid) wants premium voice, they must supply their own personal ElevenLabs API key.
```



changes: PLans if I buy the apple developer 99$



