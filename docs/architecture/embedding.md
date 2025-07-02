**Column Explanations:**

The **Model** column lists the specific name and version of each embedding model. This helps identify the precise architecture being referenced, especially important when models have multiple variants or updates.

The **Use Case** describes the main application area for the model, such as retrieval-augmented generation (RAG), image captioning, or multilingual understanding. Understanding this helps determine the suitability of the model for a given task.

**Speed** reflects the general performance and inference time of the model. Faster models are often preferred in real-time systems, while slower models may be acceptable in offline or batch processing contexts.

**Accuracy** provides a qualitative measure of how well the model performs on standard semantic similarity or retrieval benchmarks. This is critical for determining how reliable the embeddings are for tasks like search or clustering.

**Cost / License** outlines the financial or licensing model associated with using the embedding model. It differentiates between open-source models, paid APIs, and those with tiered or usage-based pricing structures.

**Vendor** indicates the organization or research lab that developed or maintains the model. This is useful for evaluating reliability, long-term support, and ecosystem integration.

**Dim** stands for the dimensionality of the output embedding vector. A higher dimensionality can encode more information but comes at a cost in terms of storage and computational efficiency.

**Context** refers to the maximum number of tokens the model can process in a single input. This is particularly relevant when embedding long documents or paragraphs.

**Multilingual** signals whether the model supports languages beyond English. This is crucial for global applications and working with diverse datasets.

**Modality** describes the type of input the model can handle — whether it processes text, image, or both. Multimodal models are more flexible but may be slower or more complex to use.

**Hardware** indicates the typical computational resources needed to run the model efficiently. Some models are lightweight and CPU-friendly, while others require GPUs or cloud infrastructure.

**Fine-tunable** states whether the model can be adapted to specific domains or tasks through additional training. This is useful for teams with custom data who want to improve performance.

**License** explains the legal terms under which the model can be used. It’s important to consider this for commercial use, redistribution, or modification.

**Last Updated** provides the date or year of the latest major release. Newer models are more likely to include architectural improvements and better training data.

**Ref URL** links to the official documentation or hosting page for the model, offering direct access to usage guides, papers, or repositories.

| Model                             | Use Case                         | Speed     | Accuracy    | Cost / License                    | Vendor          | Dim    | Context | Multilingual  | Modality   | Hardware | Fine-tunable | License     | Last Updated | Ref URL                                                                                       |
| --------------------------------- | -------------------------------- | --------- | ----------- | --------------------------------- | --------------- | ------ | ------- | ------------- | ---------- | -------- | ------------ | ----------- | ------------ | --------------------------------------------------------------------------------------------- |
| OpenAI text-embedding-3-small     | Text (RAG, search)               | Fast      | Very High   | \$0.00002 / 1K tokens (API)       | OpenAI          | 1536   | 8192    | ❌             | Text       | Cloud    | No           | Proprietary | 2024         | [Link](https://platform.openai.com/docs/guides/embeddings)                                    |
| Cohere Embed v3                   | Text, Multilingual               | Fast      | High        | Paid API, free tier available     | Cohere          | 1024   | 512     | ✅             | Text       | Cloud    | No           | Proprietary | 2024         | [Link](https://docs.cohere.com/docs/embeddings)                                               |
| Cohere Embed v4                   | Long-form, multilingual search   | Medium    | Very High   | Paid API                          | Cohere          | 1536   | 131072  | ✅             | Text       | Cloud    | No           | Proprietary | 2025         | [Link](https://docs.cohere.com/docs/embeddings-reference)                                     |
| BGE (BAAI)                        | Open source RAG                  | Medium    | High        | Free / Open Source                | BAAI            | 1024   | 512     | ❌             | Text       | CPU/GPU  | Yes          | Apache 2.0  | 2023         | [Link](https://huggingface.co/BAAI/bge-large-en)                                              |
| E5 (Google/BAAI)                  | Retrieval, QA                    | Medium    | High        | Free / Open Source                | Google/BAAI     | 1024   | 512     | ✅             | Text       | CPU/GPU  | Yes          | Apache 2.0  | 2023         | [Link](https://huggingface.co/intfloat/e5-large)                                              |
| MiniLM (all-MiniLM-L6-v2)         | General-purpose, fast            | Very Fast | Medium      | Free / Open Source                | SBERT           | 384    | 256–512 | ❌             | Text       | CPU/GPU  | No           | Apache 2.0  | 2021         | [Link](https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2)                         |
| Instructor XL                     | Task-aware embeddings            | Medium    | Very High   | Free / Open Source                | HKUNLP          | 768    | 512     | ✅             | Text       | GPU      | Yes          | Apache 2.0  | 2023         | [Link](https://huggingface.co/hkunlp/instructor-xl)                                           |
| CLIP (ViT-B/32)                   | Image+Text matching              | Medium    | Good        | Free / Open Source                | OpenAI          | 512    | 77      | ✅ (zero-shot) | Multimodal | GPU      | Yes          | MIT         | 2021         | [Link](https://huggingface.co/openai/clip-vit-base-patch32)                                   |
| BLIP-2                            | Image captioning + embeddings    | Slow      | High        | Free / Open Source                | Salesforce      | 1024   | 512     | ✅             | Multimodal | GPU      | Yes          | BSD-3       | 2023         | [Link](https://huggingface.co/Salesforce/blip2-opt-2.7b)                                      |
| GTE (Google v1)                   | Web-scale retrieval              | Fast      | High        | Free / Open Source                | Google          | 768    | 512     | ✅             | Text       | CPU/GPU  | No           | Apache 2.0  | 2023         | [Link](https://huggingface.co/thenlper/gte-large)                                             |
| GTE v1.5 (Alibaba)                | Large context retrieval          | Fast      | High        | Free / Open Source                | Alibaba DAMO    | 768    | 8192    | ✅             | Text       | CPU/GPU  | No           | Apache 2.0  | 2024         | [Link](https://huggingface.co/thenlper/gte-large-en-v1.5)                                     |
| LaBSE                             | Multilingual sentence embeddings | Medium    | High        | Free / Open Source                | Google          | 768    | 512     | ✅             | Text       | CPU/GPU  | No           | Apache 2.0  | 2020         | [Link](https://huggingface.co/sentence-transformers/LaBSE)                                    |
| MPNet                             | General text understanding       | Medium    | Medium-High | Free / Open Source                | Microsoft/SBERT | 768    | 384–512 | ❌             | Text       | CPU/GPU  | No           | Apache 2.0  | 2021         | [Link](https://huggingface.co/sentence-transformers/all-mpnet-base-v2)                        |
| DeepLake (Activeloop)             | Embedding + storage infra        | Medium    | Varies      | Free tier + storage-based pricing | Activeloop      | -      | -       | ✅             | Multimodal | CPU/GPU  | N/A          | MPL v2.0    | 2024         | [Link](https://docs.deeplake.ai/en/latest/integrations/llm/embedding.html)                    |
| Gemma Embedding (Google)          | Text embeddings from LLMs        | Fast      | SOTA        | Free / Open Source                | Google DeepMind | 3072   | 128000  | ❌             | Text       | GPU      | Yes          | Apache 2.0  | 2025         | [Link](https://huggingface.co/google/gemma-2b-it)                                             |
| textembedding-gecko\@001 (Google) | General embedding API            | Fast      | High        | API via Vertex AI                 | Google Cloud    | 768    | 3072    | ✅             | Text       | Cloud    | No           | Proprietary | 2025         | [Link](https://cloud.google.com/vertex-ai/docs/generative-ai/embeddings/get-text-embeddings)  |
| gemini-embedding-001 (Google)     | Unified multimodal embedding     | Fast      | High        | API via Vertex AI                 | Google DeepMind | Varies | \~2048  | ✅             | Multimodal | Cloud    | No           | Proprietary | 2025         | [Link](https://cloud.google.com/vertex-ai/generative-ai/docs/learn/model-versioning-overview) |
