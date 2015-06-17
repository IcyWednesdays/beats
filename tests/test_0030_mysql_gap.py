from pbtests.packetbeat import TestCase

"""
Tests for MySQL messages with gaps (packet loss) in them.
"""


class Test(TestCase):

    def test_gap_in_large_file(self):
        """
        Should recover well from losing a packet in a large
        response.
        """
        self.render_config_template(
            mysql_ports=[3306],
        )
        self.run_packetbeat(pcap="mysql_with_gap.pcap")

        objs = self.read_output()
        assert len(objs) == 1
        o = objs[0]

        assert o["status"] == "OK"
        assert o["method"] == "SELECT"
        assert o["mysql.num_rows"] > 1
