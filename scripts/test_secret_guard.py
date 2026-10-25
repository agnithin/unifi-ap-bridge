import unittest
from check_secrets import findings
class SecretGuardTests(unittest.TestCase):
    def test_literal_and_escaped_headers(self):
        token="Synthetic"+"Fixture"*4
        for text in ["X-API-Key: "+token,'{"X-API-KEY": "'+token+'"}',r'{\"X-API-KEY\": \"'+token+r'\"}']:
            self.assertEqual(findings(text),[1])
    def test_environment_and_templates(self):
        for text in ['{"X-API-KEY": os.environ["CONTROLLER_API_KEY"]}',"X-API-KEY: $CONTROLLER_API_KEY","X-API-KEY: <redacted>"]:
            self.assertEqual(findings(text),[])
if __name__=="__main__":unittest.main()
